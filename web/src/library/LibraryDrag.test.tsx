import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { dropOn, layoutCards } from "../testing/cardDrag";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

// ライブラリで動画を動画に落とす（specs/050-drag-organize/ui-design.md「Drop menus」「The new
// group's name」「Library」、受け入れ条件 2、3）。

function video(id: number, title: string): Video {
  return {
    id,
    title,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
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

interface Write {
  method: string;
  path: string;
  body: unknown;
}

const server = {
  items: [] as LibraryItem[],
  writes: [] as Write[],
  /** true の間、束ねる要求は 400 too_many_videos で失敗する。 */
  tooManyVideos: false,
};

/** organize は「Make a group」と「Bundle as versions」に契約どおり答える。 */
function organize(path: string, body: unknown): Response | undefined {
  if (path === "/api/organize/group") {
    const { items, target } = body as { items: { id: number }[]; target: number };
    const lifted = new Set(items.map((each) => each.id));
    const targetItem = server.items.find(
      (item) => item.kind === "video" && item.video.id === target,
    );
    const name = targetItem?.kind === "video" ? targetItem.video.title : "";
    const made = group(90, name, [target, ...lifted]);
    server.items = server.items.flatMap((item): LibraryItem[] => {
      if (item.kind !== "video") return [item];
      if (item.video.id === target) return [{ kind: "group", group: made }];
      return lifted.has(item.video.id) ? [] : [item];
    });
    return json({ undoToken: "token-group", groupId: 90 });
  }
  if (path === "/api/organize/bundle" && server.tooManyVideos) {
    return json(
      { code: "invalid_request", reason: "too_many_videos", limit: 20, message: "x" },
      400,
    );
  }
  if (path === "/api/organize/bundle") {
    const { videoIds } = body as { videoIds: number[] };
    server.items = server.items.filter(
      (item) => item.kind !== "video" || !videoIds.includes(item.video.id),
    );
    return json({ undoToken: "token-bundle" });
  }
  if (path === "/api/groups/90") {
    const { name } = body as { name: string };
    server.items = server.items.map((item) =>
      item.kind === "group" && item.group.id === 90
        ? { kind: "group", group: { ...item.group, name } }
        : item,
    );
    return json(group(90, name, []));
  }
  return undefined;
}

const fetchMock = vi.fn<typeof fetch>((input, init) => {
  const url = new URL(String(input), "http://localhost");
  const method = init?.method ?? "GET";
  if (method !== "GET") {
    const body = JSON.parse(String(init?.body)) as unknown;
    server.writes.push({ method, path: url.pathname, body });
    const answer = organize(url.pathname, body);
    if (answer !== undefined) return Promise.resolve(answer);
  }
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
  throw new Error(`unexpected request: ${method} ${url.toString()}`);
});

function renderLibrary() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/** cardTitles は格子のカードの題名か名前の欄の値を左から返す。 */
function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) =>
      card.querySelector("h3")?.textContent ?? card.querySelector("input")?.value ?? "",
  );
}

/** open はライブラリを開き、動画 A、B、C を左から並べる。 */
async function open() {
  server.items = [
    { kind: "video", video: video(1, "A") },
    { kind: "video", video: video(2, "B") },
    { kind: "video", video: video(3, "C") },
  ];
  layoutCards();
  renderLibrary();
  await screen.findByRole("link", { name: "C" });
}

/** dropAOnB は A を B に落とし、開いたメニューを返す。 */
async function dropAOnB() {
  dropOn("A", 0, 1);
  return screen.findByRole("menu");
}

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  server.writes = [];
  server.tooManyVideos = false;
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
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("LibraryPage drag a video onto a video", () => {
  it("A を B に落とすと「Dropped on B」に 2 つの項目を出し、「Make a group」は B の位置に B の名前のグループを 1 枚出す", async () => {
    await open();
    const menu = await dropAOnB();

    expect(within(menu).getByText('Dropped on "B"')).toBeDefined();
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["Make a group", "Bundle as versions"]);
    await userEvent
      .setup()
      .click(within(menu).getByRole("menuitem", { name: "Make a group" }));

    expect(cardTitles()).toEqual(["B", "C"]);
    await waitFor(() =>
      expect(server.writes).toEqual([
        {
          method: "POST",
          path: "/api/organize/group",
          body: { items: [{ kind: "video", id: 1 }], target: 2 },
        },
      ]),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    expect((field as HTMLInputElement).value).toBe("B");
    expect(document.activeElement).toBe(field);
    expect(await screen.findByText('Made the group "B"')).toBeDefined();
  });

  it("名前の欄で変えた名前の Enter は、「Make a group」のトークン付きで名前を変える", async () => {
    await open();
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    await user.clear(field);
    await user.type(field, "Trip{Enter}");

    await waitFor(() =>
      expect(server.writes.at(-1)).toEqual({
        method: "PATCH",
        path: "/api/groups/90",
        body: { name: "Trip", undoToken: "token-group" },
      }),
    );
    expect(await screen.findByRole("heading", { name: "Trip" })).toBeDefined();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("「Bundle as versions」は B を代表に束ねて A を消し、ダイアログを開かない", async () => {
    await open();
    await userEvent
      .setup()
      .click(
        within(await dropAOnB()).getByRole("menuitem", { name: "Bundle as versions" }),
      );

    expect(cardTitles()).toEqual(["B", "C"]);
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() =>
      expect(server.writes).toEqual([
        {
          method: "POST",
          path: "/api/organize/bundle",
          body: { videoIds: [1], target: 2 },
        },
      ]),
    );
    expect(await screen.findByText('Bundled 2 videos as versions of "B"')).toBeDefined();
  });

  it("束ねて 20 本を超えると持ち上げたカードを戻し、理由を出して取り消しは出さない", async () => {
    await open();
    server.tooManyVideos = true;
    await userEvent
      .setup()
      .click(
        within(await dropAOnB()).getByRole("menuitem", { name: "Bundle as versions" }),
      );

    expect(
      await screen.findByText("Couldn't bundle: Select between 1 and 20 videos."),
    ).toBeDefined();
    expect(cardTitles()).toEqual(["A", "B", "C"]);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("Esc でメニューを閉じると何も送らず、A と B はそのまま", async () => {
    await open();
    await dropAOnB();
    await userEvent.setup().keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(cardTitles()).toEqual(["A", "B", "C"]);
    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
  });
});
