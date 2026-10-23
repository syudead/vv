import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { OrganizableItem as LibraryItem } from "../api/libraryItems";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import {
  afterDrop,
  layoutCards,
  lift,
  mouse,
  moveTo,
  openedMenu,
  release,
} from "../testing/cardDrag";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

// jsdom has no layout; place 200px cards in one row for pointer drag tests.

function video(id: number): Video {
  return {
    id,
    virtual: false,
    title: `Clip ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    fileModifiedAt: "2026-09-01T00:00:00Z",
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
  moveAnswer: (() => Response) | null;
  undo: Map<string, LibraryItem[]>;
  hold: Promise<void> | null;
}

const fetchMock = vi.fn<typeof fetch>();

function move(server: Server, body: { items: { kind: string; id: number }[] }) {
  if (server.moveAnswer !== null) return server.moveAnswer();
  const token = `token-${body.items.map((ref) => String(ref.id)).join(",")}`;
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
  const server: Server = {
    items,
    writes: [],
    moveAnswer: null,
    undo: new Map(),
    hold: null,
  };
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
    if (url.pathname === "/api/organize/merge") {
      server.writes.push({ path: url.pathname, body });
      if (server.moveAnswer !== null) return Promise.resolve(server.moveAnswer());
      return Promise.resolve(move(server, body as Parameters<typeof move>[1]));
    }
    if (url.pathname === "/api/organize/move") {
      server.writes.push({ path: url.pathname, body });
      const hold = server.hold ?? Promise.resolve();
      server.hold = null;
      return hold.then(() => move(server, body as Parameters<typeof move>[1]));
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

function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) => card.querySelector("h3")?.textContent ?? "",
  );
}

function held(server: Server): () => Promise<void> {
  let open = () => {};
  server.hold = new Promise((resolve) => {
    open = resolve;
  });
  return async () => {
    open();
    await act(async () => {
      await Promise.resolve();
    });
  };
}

function moves(server: Server) {
  return server.writes
    .filter((each) => each.path === "/api/organize/move")
    .map((each) => each.body);
}

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
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    try {
      release(1);
      // dnd-kit suppresses document clicks for 50ms after a drop.
      await act(() => vi.advanceTimersByTimeAsync(50));
    } finally {
      vi.useRealTimers();
    }
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

  it("前の move が答えるまで次の move を送らず、Undo は最後に落とした移動を取り消す", async () => {
    const server = library();
    await openLibrary();
    const open = held(server);
    lift("Clip 1", 0);
    moveTo(1);
    release(1);
    lift("Clip 2", 1);
    moveTo(0);
    release(0);

    expect(cardTitles()).toEqual(["Trip", "Clip 3"]);
    await act(async () => {
      await Promise.resolve();
    });
    expect(moves(server)).toEqual([{ items: [{ kind: "video", id: 1 }], to: 7 }]);

    await open();
    await waitFor(() =>
      expect(moves(server)).toEqual([
        { items: [{ kind: "video", id: 1 }], to: 7 },
        { items: [{ kind: "video", id: 2 }], to: 7 },
      ]),
    );
    await waitFor(() => expect(server.items).toHaveLength(2));
    fireEvent.click(await screen.findByRole("button", { name: "Undo" }));

    await screen.findByText("Undone");
    expect(server.writes.at(-1)?.body).toEqual({ token: "token-2" });
  });

  it("取り消しても、答えを待っている移動のカードは隠したままにする", async () => {
    const server = library();
    await openLibrary();
    lift("Clip 1", 0);
    moveTo(1);
    release(1);
    await screen.findByText('Moved 1 item to "Trip"');
    await waitFor(() => expect(cardTitles()).toEqual(["Trip", "Clip 2", "Clip 3"]));

    const open = held(server);
    lift("Clip 2", 1);
    moveTo(0);
    release(0);
    expect(cardTitles()).toEqual(["Trip", "Clip 3"]);
    await afterDrop();
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    await screen.findByText("Undone");
    await waitFor(() => expect(cardTitles()).toEqual(["Clip 1", "Trip", "Clip 3"]));
    await open();
    await waitFor(() => expect(moves(server)).toHaveLength(2));
  });

  it("答えを待っている移動で隠れたカードは、Shift の範囲に入らない", async () => {
    const server = library();
    await openLibrary();
    const open = held(server);
    lift("Clip 2", 2);
    moveTo(1);
    release(1);
    expect(cardTitles()).toEqual(["Clip 1", "Trip", "Clip 3"]);
    await afterDrop();

    fireEvent.click(screen.getByRole("checkbox", { name: 'Select "Clip 1"' }));
    const last = screen.getByRole("checkbox", { name: 'Select "Clip 3"' });
    fireEvent.pointerDown(last, { ...mouse, shiftKey: true });
    fireEvent.click(last, { shiftKey: true });
    expect(screen.getByText("4 videos selected")).toBeDefined();
    await open();
  });

  it("ライブラリを離れた後に失敗した move は、行った先で知らせない", async () => {
    const server = library();
    server.moveAnswer = () => json({ code: "internal", message: "x" }, 500);
    await openLibrary();
    const open = held(server);
    lift("Clip 1", 0);
    moveTo(1);
    release(1);
    await waitFor(() => expect(moves(server)).toHaveLength(1));
    await afterDrop();

    fireEvent.click(screen.getByRole("link", { name: "Clip 3" }));
    await screen.findByText("Player");
    await open();

    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByText(/^Couldn't move:/)).toBeNull();
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

describe("LibraryPage drop a group on a group", () => {
  async function openTwoGroups() {
    layoutCards();
    renderLibrary();
    await screen.findByRole("link", { name: "Clip 2" });
  }

  function twoGroups() {
    return install([
      { kind: "video", video: video(1) },
      { kind: "group", group: group(7, "Trip", [20, 21]) },
      { kind: "group", group: group(8, "Beach", [30]) },
      { kind: "video", video: video(2) },
    ]);
  }

  function merges(server: Server) {
    return server.writes
      .filter((each) => each.path === "/api/organize/merge")
      .map((each) => each.body);
  }

  async function dropTripOnBeach() {
    lift(/^Trip/, 1);
    moveTo(2);
    expect(screen.getByText("Put in or merge…")).toBeDefined();
    release(2);
    const menu = await openedMenu();
    expect(within(menu).getByText('Dropped on "Beach"')).toBeDefined();
    return menu;
  }

  it("マージした後も一覧に残る動画（ほかのグループにも入っている）は、読み直しで見せる", async () => {
    const server = twoGroups();
    // Membership in another group can keep this video at the top level after adding it.
    //
    server.moveAnswer = () => {
      server.items = server.items.filter(
        (item) => item.kind !== "group" || item.group.id !== 7,
      );
      return json({ undoToken: "token-kept" });
    };
    await openTwoGroups();
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: 'Select "Clip 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "Trip"' }));
    const menu = await dropTripOnBeach();

    await user.click(within(menu).getByRole("menuitem", { name: "Merge" }));

    expect(await screen.findByText('Merged 2 items into "Beach"')).toBeDefined();
    expect(merges(server)).toEqual([
      {
        items: [
          { kind: "video", id: 1 },
          { kind: "group", id: 7 },
        ],
        into: 8,
      },
    ]);
    await waitFor(() => expect(cardTitles()).toEqual(["Clip 1", "Beach", "Clip 2"]));
  });

  it("「Merge」は into の merge を送り、マージしたグループのカードが消える", async () => {
    const server = twoGroups();
    await openTwoGroups();
    const user = userEvent.setup();
    const menu = await dropTripOnBeach();
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["Put in as a child group", "Merge"]);

    await user.click(within(menu).getByRole("menuitem", { name: "Merge" }));

    expect(cardTitles()).toEqual(["Clip 1", "Beach", "Clip 2"]);
    await waitFor(() =>
      expect(merges(server)).toEqual([{ items: [{ kind: "group", id: 7 }], into: 8 }]),
    );
    expect(await screen.findByText('Merged 1 group into "Beach"')).toBeDefined();
    expect(moves(server)).toEqual([]);
  });

  it("「Put in as a child group」は to の move を送る", async () => {
    const server = twoGroups();
    await openTwoGroups();
    const user = userEvent.setup();
    const menu = await dropTripOnBeach();

    await user.click(
      within(menu).getByRole("menuitem", { name: "Put in as a child group" }),
    );

    expect(cardTitles()).toEqual(["Clip 1", "Beach", "Clip 2"]);
    await waitFor(() =>
      expect(moves(server)).toEqual([{ items: [{ kind: "group", id: 7 }], to: 8 }]),
    );
    expect(merges(server)).toEqual([]);
  });

  it.each([
    ["Merge", "/api/organize/merge"],
    ["Put in as a child group", "/api/organize/move"],
  ] as const)(
    "動画とグループの選択で「%s」を選ぶと、すべてのカードを画面の順で 1 回の要求に送る",
    async (choice, path) => {
      const server = twoGroups();
      await openTwoGroups();
      const user = userEvent.setup();
      await user.click(screen.getByRole("checkbox", { name: 'Select "Clip 2"' }));
      await user.click(screen.getByRole("checkbox", { name: 'Select the group "Trip"' }));
      await user.click(screen.getByRole("checkbox", { name: 'Select "Clip 1"' }));
      const menu = await dropTripOnBeach();

      await user.click(within(menu).getByRole("menuitem", { name: choice }));

      expect(cardTitles()).toEqual(["Beach"]);
      await waitFor(() => expect(server.writes).toHaveLength(1));
      const items = [
        { kind: "video", id: 1 },
        { kind: "group", id: 7 },
        { kind: "video", id: 2 },
      ];
      expect(server.writes[0]).toEqual({
        path,
        body: path === "/api/organize/merge" ? { items, into: 8 } : { items, to: 8 },
      });
      expect(server.writes[0]?.body).not.toHaveProperty("from");
      expect(
        await screen.findByText(
          choice === "Merge" ? 'Merged 3 items into "Beach"' : 'Moved 3 items to "Beach"',
        ),
      ).toBeDefined();
    },
  );

  it("Esc でメニューを閉じると何も送らず、カードは元のまま", async () => {
    const server = twoGroups();
    await openTwoGroups();
    const user = userEvent.setup();
    await dropTripOnBeach();

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Clip 1", "Trip", "Beach", "Clip 2"]);
    expect(document.querySelector("[data-drag-card].opacity-50")).toBeNull();
  });

  it("Undo でマージしたグループのカードが戻る", async () => {
    const server = twoGroups();
    await openTwoGroups();
    const user = userEvent.setup();
    const menu = await dropTripOnBeach();
    await user.click(within(menu).getByRole("menuitem", { name: "Merge" }));
    await screen.findByText('Merged 1 group into "Beach"');

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(await screen.findByText("Undone")).toBeDefined();
    expect(server.writes.map((each) => each.path)).toEqual([
      "/api/organize/merge",
      "/api/organize/undo",
    ]);
    await waitFor(() =>
      expect(cardTitles()).toEqual(["Clip 1", "Trip", "Beach", "Clip 2"]),
    );
  });

  it("失敗した merge は消したカードを戻し、Undo を出さずに知らせる", async () => {
    const server = twoGroups();
    server.moveAnswer = () => json({ code: "internal", message: "x" }, 500);
    await openTwoGroups();
    const user = userEvent.setup();
    const menu = await dropTripOnBeach();
    await user.click(within(menu).getByRole("menuitem", { name: "Merge" }));

    expect(await screen.findByText(/^Couldn't merge:/)).toBeDefined();
    expect(cardTitles()).toEqual(["Clip 1", "Trip", "Beach", "Clip 2"]);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });
});
