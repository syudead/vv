import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

// ライブラリでグループのカードへ落とす（specs/050-drag-organize/ui-design.md「Library」
// 「What a drop does」）。jsdom は配置を持たないので、格子のカードを横一列に 200px ずつ置き、
// ポインタの出来事で持ち上げて落とす。

function video(id: number): Video {
  return {
    id,
    title: `Clip ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "pending",
    tags: [],
    favorite: false,
  };
}

function group(id: number, name: string, videoIds: number[]): LibraryGroup {
  return {
    id,
    name,
    videoCount: videoIds.length,
    watchedCount: 0,
    watchState: "unwatched",
    durationMs: videoIds.length * 60_000,
    sizeBytes: 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: [],
    openVideoId: videoIds[0] ?? 0,
    videoIds,
    tags: [],
    favorite: false,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

interface Server {
  items: LibraryItem[];
  writes: { path: string; body: unknown }[];
  /** 置くと POST /api/organize/move がこの応答を返し、何も変えない。 */
  moveAnswer: (() => Response) | null;
  /** 取り消しのトークンごとの、操作の前の一覧。 */
  undo: Map<string, LibraryItem[]>;
}

const fetchMock = vi.fn<typeof fetch>();

/** move は POST /api/organize/move を受け、動かした項目を一覧から外す。 */
function move(server: Server, body: { items: { kind: string; id: number }[] }) {
  if (server.moveAnswer !== null) return server.moveAnswer();
  const token = `token-${String(server.undo.size + 1)}`;
  server.undo.set(token, server.items);
  server.items = server.items.filter(
    (item) =>
      !body.items.some((ref) =>
        item.kind === "video"
          ? ref.kind === "video" && ref.id === item.video.id
          : ref.kind === "group" && ref.id === item.group.id,
      ),
  );
  return json({ undoToken: token });
}

function install(items: LibraryItem[]): Server {
  const server: Server = { items, writes: [], moveAnswer: null, undo: new Map() };
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined;
    if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
    if (url.pathname === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/library") {
      return Promise.resolve(json({ items: server.items, total: server.items.length }));
    }
    if (url.pathname === "/api/organize/move") {
      server.writes.push({ path: url.pathname, body });
      return Promise.resolve(move(server, body as Parameters<typeof move>[1]));
    }
    if (url.pathname === "/api/organize/undo") {
      server.writes.push({ path: url.pathname, body });
      const before = server.undo.get((body as { token: string }).token);
      if (before !== undefined) server.items = before;
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
  return server;
}

function renderLibrary(audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
                <Route path="/videos/:id" element={<p>Player</p>} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const cardWidth = 190;
const cardHeight = 150;
const step = 200;

/** layoutCards は格子のカード（ドラッグの包み）を左から 200px ずつ、同じ高さに置く。 */
function layoutCards() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const card = this.closest("[data-drag-card]");
    const index =
      card === null ? 0 : Array.from(card.parentElement?.children ?? []).indexOf(card);
    const left = index * step;
    return {
      x: left,
      y: 0,
      top: 0,
      left,
      right: left + cardWidth,
      bottom: cardHeight,
      width: card === null ? 0 : cardWidth,
      height: card === null ? 0 : cardHeight,
      toJSON: () => ({}),
    } as DOMRect;
  });
}

const mouse = { pointerType: "mouse", isPrimary: true, button: 0, pointerId: 1 };

function pointOn(index: number, ratio: number) {
  return { clientX: index * step + cardWidth * ratio, clientY: cardHeight / 2 };
}

/** lift はカード `title` のリンクを押し、持ち上がるまで動かす。 */
function lift(title: string | RegExp, index: number) {
  const link = screen.getByRole("link", { name: title });
  fireEvent.pointerDown(link, { ...mouse, ...pointOn(index, 0.5) });
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, 0.4) });
}

function moveTo(index: number, ratio = 0.5) {
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, ratio) });
}

function release(index: number, ratio = 0.5) {
  fireEvent.pointerUp(document, { ...mouse, ...pointOn(index, ratio) });
}

function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) => card.querySelector("h3")?.textContent ?? "",
  );
}

function moves(server: Server) {
  return server.writes
    .filter((each) => each.path === "/api/organize/move")
    .map((each) => each.body);
}

/** library は「Clip 1」「Trip」（グループ 7）「Clip 2」「Clip 3」を並べる。 */
function library() {
  return install([
    { kind: "video", video: video(1) },
    { kind: "group", group: group(7, "Trip", [20, 21]) },
    { kind: "video", video: video(2) },
    { kind: "video", video: video(3) },
  ]);
}

async function openLibrary() {
  layoutCards();
  renderLibrary();
  await screen.findByRole("link", { name: "Clip 3" });
}

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  cleanup();
  fetchMock.mockReset();
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("LibraryPage drag into a group", () => {
  it("グループのカードに落とした動画は to 付きの move を送り、すぐ一覧から外れる", async () => {
    const server = library();
    await openLibrary();

    lift("Clip 1", 0);
    moveTo(1);
    expect(screen.getByText("Put in")).toBeDefined();
    release(1);

    expect(cardTitles()).toEqual(["Trip", "Clip 2", "Clip 3"]);
    await waitFor(() =>
      expect(moves(server)).toEqual([{ items: [{ kind: "video", id: 1 }], to: 7 }]),
    );
    expect(await screen.findByText('Moved 1 item to "Trip"')).toBeDefined();
  });

  it("選択した 3 枚は、画面の順で 3 つの参照を送り、選択は解ける", async () => {
    const server = library();
    await openLibrary();
    const user = userEvent.setup();
    for (const id of [3, 1, 2]) {
      await user.click(
        screen.getByRole("checkbox", { name: `Select "Clip ${String(id)}"` }),
      );
    }

    lift("Clip 2", 2);
    moveTo(1);
    expect(screen.getByLabelText("3 items").textContent).toBe("3");
    release(1);

    expect(cardTitles()).toEqual(["Trip"]);
    await waitFor(() =>
      expect(moves(server)).toEqual([
        {
          items: [
            { kind: "video", id: 1 },
            { kind: "video", id: 2 },
            { kind: "video", id: 3 },
          ],
          to: 7,
        },
      ]),
    );
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });

  it("グループのカードを自分の上に持っていくと拒否の印を出し、落としても何も送らない", async () => {
    const server = library();
    await openLibrary();

    lift(/^Trip/, 1);
    moveTo(1);
    expect(screen.getByLabelText("Can't drop here")).toBeDefined();
    release(1);

    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Clip 1", "Trip", "Clip 2", "Clip 3"]);
  });

  it("動画のカードの上は落とす先ではなく、落としても何も送らない", async () => {
    const server = library();
    await openLibrary();

    lift("Clip 1", 0);
    moveTo(2);
    expect(screen.queryByText("Put in")).toBeNull();
    release(2);

    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
  });

  it("Undo を押すと undo を送り、カードが元に戻る", async () => {
    const server = library();
    await openLibrary();
    lift("Clip 1", 0);
    moveTo(1);
    release(1);
    await screen.findByText('Moved 1 item to "Trip"');
    await waitFor(() => expect(cardTitles()).toEqual(["Trip", "Clip 2", "Clip 3"]));

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(await screen.findByText("Undone")).toBeDefined();
    expect(server.writes.map((each) => each.path)).toEqual([
      "/api/organize/move",
      "/api/organize/undo",
    ]);
    await waitFor(() =>
      expect(cardTitles()).toEqual(["Clip 1", "Trip", "Clip 2", "Clip 3"]),
    );
  });

  it("失敗した move は外したカードを戻し、Undo を出さずに知らせる", async () => {
    const server = library();
    server.moveAnswer = () => json({ code: "internal", message: "x" }, 500);
    await openLibrary();
    lift("Clip 1", 0);
    moveTo(1);
    release(1);

    expect(await screen.findByText(/^Couldn't move:/)).toBeDefined();
    expect(cardTitles()).toEqual(["Clip 1", "Trip", "Clip 2", "Clip 3"]);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("リストの行も、グループの行に落とすと to 付きの move を送る", async () => {
    const server = library();
    localStorage.setItem(
      "vv.view.v2",
      JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
    );
    vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
      this: Element,
    ) {
      const row = this.closest("tbody tr");
      const index =
        row === null ? 0 : Array.from(row.parentElement?.children ?? []).indexOf(row);
      const top = index * 48;
      const size = row === null ? 0 : 48;
      return {
        x: 0,
        y: top,
        top,
        left: 0,
        right: 800,
        bottom: top + size,
        width: row === null ? 0 : 800,
        height: size,
        toJSON: () => ({}),
      } as DOMRect;
    });
    renderLibrary();
    const link = await screen.findByRole("link", { name: "Clip 1" });
    const at = (row: number) => ({ clientX: 400, clientY: row * 48 + 24 });

    fireEvent.pointerDown(link, { ...mouse, ...at(0) });
    fireEvent.pointerMove(document, { ...mouse, clientX: 400, clientY: 34 });
    fireEvent.pointerMove(document, { ...mouse, ...at(1) });
    fireEvent.pointerUp(document, { ...mouse, ...at(1) });

    await waitFor(() =>
      expect(moves(server)).toEqual([{ items: [{ kind: "video", id: 1 }], to: 7 }]),
    );
  });

  it("ゲストにはドラッグが無い", async () => {
    const server = library();
    layoutCards();
    renderLibrary("guest");
    await screen.findByRole("link", { name: "Clip 3" });

    lift("Clip 1", 0);
    moveTo(1);
    release(1);

    await act(async () => {
      await Promise.resolve();
    });
    expect(document.querySelector("[data-drag-card]")).toBeNull();
    expect(server.writes).toEqual([]);
  });
});
