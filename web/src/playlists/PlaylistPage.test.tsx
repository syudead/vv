import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { PlaylistItem } from "../api/playlists";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import PlaylistPage from "./PlaylistPage";
import { itemTitle } from "./playlistItems";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `Video ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    container: "mp4",
    videoCodec: "h264",
    tags: [],
    ...extra,
  };
}

function group(name: string, openVideoId: number): LibraryGroup {
  return {
    id: openVideoId,
    folder: { rootId: 1, path: name },
    name,
    videoCount: 12,
    watchedCount: 3,
    watchState: "inProgress",
    durationMs: 600_000,
    sizeBytes: 2048,
    addedAt: "2026-09-01T00:00:00Z",
    previews: [],
    openVideoId,
    videoIds: [openVideoId],
    tags: [],
  };
}

/** Zero-video folders have neither a playback target nor duration. */
function folderItem(id: number, videoCount: number): PlaylistItem {
  return {
    id,
    kind: "smartFolder",
    smartFolder: {
      id: 9,
      name: "タグ: 猫",
      videoCount,
      watchedCount: 1,
      watchState: videoCount === 0 ? "unwatched" : "inProgress",
      sizeBytes: 2048,
      previews:
        videoCount === 0
          ? []
          : [{ videoId: 31, thumbnailUrl: "/api/videos/31/thumbnail" }],
      tags: [],
      ...(videoCount === 0 ? {} : { openVideoId: 31, durationMs: 120_000 }),
    },
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function failure(status: number): Response {
  return json({ code: status === 404 ? "not_found" : "internal", message: "x" }, status);
}

const server = {
  name: "Morning",
  items: [] as PlaylistItem[],
  getStatus: 200,
  getCalls: 0,
  /** Capture GET content on request, then gate delivery. */
  getHold: null as Promise<void> | null,
  moves: [] as { itemId: number; afterItemId: number | null }[],
  moveStatus: 204,
  removes: [] as number[],
  removeStatus: 204,
  renames: [] as unknown[],
  deletes: 0,
  deleteStatus: 204,
  deleteHold: null as Promise<void> | null,
  /** Hold GET for playlist 8 indefinitely. */
  otherGets: 0,
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/playlists/7" && method === "GET") {
      server.getCalls += 1;
      const response =
        server.getStatus !== 200
          ? failure(server.getStatus)
          : json({
              id: 7,
              name: server.name,
              itemCount: server.items.length,
              updatedAt: "2026-09-01T00:00:00Z",
              items: server.items,
            });
      return (server.getHold ?? Promise.resolve()).then(() => response);
    }
    const move = /^\/api\/playlists\/7\/items\/(\d+)\/move$/.exec(url);
    if (move !== null && method === "POST") {
      const itemId = Number(move[1]);
      const { afterItemId } = JSON.parse(String(init?.body)) as {
        afterItemId: number | null;
      };
      server.moves.push({ itemId, afterItemId });
      if (server.moveStatus !== 204) return Promise.resolve(failure(server.moveStatus));
      const moving = server.items.find((item) => item.id === itemId);
      const rest = server.items.filter((item) => item.id !== itemId);
      const at =
        afterItemId === null ? 0 : rest.findIndex((i) => i.id === afterItemId) + 1;
      if (moving !== undefined) rest.splice(at, 0, moving);
      server.items = rest;
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    const item = /^\/api\/playlists\/7\/items\/(\d+)$/.exec(url);
    if (item !== null && method === "DELETE") {
      const itemId = Number(item[1]);
      server.removes.push(itemId);
      if (server.removeStatus !== 204)
        return Promise.resolve(failure(server.removeStatus));
      server.items = server.items.filter((each) => each.id !== itemId);
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url === "/api/playlists/7" && method === "PATCH") {
      const body = JSON.parse(String(init?.body)) as { name: string };
      server.renames.push(body);
      server.name = body.name;
      return Promise.resolve(
        json({ id: 7, name: body.name, itemCount: 0, updatedAt: "2026-09-02T00:00:00Z" }),
      );
    }
    if (url === "/api/playlists/7" && method === "DELETE") {
      server.deletes += 1;
      return (server.deleteHold ?? Promise.resolve()).then(() =>
        server.deleteStatus === 204
          ? new Response(null, { status: 204 })
          : failure(server.deleteStatus),
      );
    }
    if (url === "/api/playlists/8" && method === "GET") {
      server.otherGets += 1;
      return new Promise<Response>(() => undefined);
    }
    throw new Error(`unexpected request: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

function VideoProbe() {
  const location = useLocation();
  return (
    <p>
      {`Video page ${location.pathname}${location.search} ${JSON.stringify(location.state)}`}
    </p>
  );
}

let navigateTo: (to: string) => void = () => undefined;

function NavigateProbe() {
  const navigate = useNavigate();
  navigateTo = (to) => void navigate(to);
  return null;
}

function renderPage(path = "/playlists/7") {
  render(
    <MemoryRouter initialEntries={[path]}>
      <TooltipProvider>
        <ToastProvider>
          <NavigateProbe />
          <Routes>
            <Route path="/playlists" element={<p>Playlists screen</p>} />
            <Route path="/playlists/:id" element={<PlaylistPage />} />
            <Route path="/videos/:id" element={<VideoProbe />} />
            <Route path="/" element={<p>Library</p>} />
          </Routes>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function hold(): () => void {
  let release: () => void = () => undefined;
  server.getHold = new Promise<void>((done) => {
    release = done;
  });
  return () => {
    server.getHold = null;
    release();
  };
}

function serverNames(): string[] {
  return server.items.map((item) => `Play from ${itemTitle(item)}`);
}

function rowNames(): string[] {
  const table = screen.getByRole("table", { name: "Items" });
  return within(table)
    .getAllByRole("link")
    .map((link) => link.getAttribute("aria-label") ?? "");
}

// Provide 48px row geometry because dnd-kit keyboard navigation needs layout absent from jsdom.
function layoutRows() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const row = this.closest("tr[data-item-id]");
    const index =
      row === null ? 0 : Array.from(row.parentElement?.children ?? []).indexOf(row);
    const top = index * 48;
    return {
      x: 0,
      y: top,
      top,
      left: 0,
      right: 800,
      bottom: top + 48,
      width: 800,
      height: 48,
      toJSON: () => ({}),
    } as DOMRect;
  });
}

beforeEach(() => {
  server.name = "Morning";
  server.items = [
    {
      id: 1,
      kind: "video",
      video: video(11, {
        progress: {
          positionMs: 1000,
          completed: true,
          updatedAt: "2026-09-01T00:00:00Z",
        },
      }),
    },
    { id: 2, kind: "group", group: group("Series", 21) },
    { id: 3, kind: "video", video: video(13) },
  ];
  server.getStatus = 200;
  server.getCalls = 0;
  server.getHold = null;
  server.moves = [];
  server.moveStatus = 204;
  server.removes = [];
  server.removeStatus = 204;
  server.renames = [];
  server.deletes = 0;
  server.deleteStatus = 204;
  server.deleteHold = null;
  server.otherGets = 0;
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("PlaylistPage", () => {
  it("項目を応答の順に、それぞれの視聴状態とともに並べる", async () => {
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    expect(screen.getByRole("heading", { level: 1, name: "Morning" })).toBeDefined();
    await waitFor(() => expect(document.title).toBe("Morning"));
    expect(screen.getByText("3 items · 12:00")).toBeDefined();
    expect(rowNames()).toEqual([
      "Play from Video 11",
      "Play from Series",
      "Play from Video 13",
    ]);
    const rows = within(screen.getByRole("table", { name: "Items" })).getAllByRole("row");
    expect(within(rows[0]!).getByLabelText("Watched")).toBeDefined();
    expect(within(rows[1]!).getByText("3 / 12")).toBeDefined();
    expect(within(rows[1]!).getByText("12 videos")).toBeDefined();
    expect(within(rows[2]!).queryByLabelText("Watched")).toBeNull();
  });

  it("「Play」は先頭の項目から、3 行目はその項目から、グループは openVideoId を開く", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    expect(
      screen
        .getByRole("link", { name: 'Play "Morning" from the top' })
        .getAttribute("href"),
    ).toBe("/videos/11?playlist=7&item=1");
    expect(
      screen.getByRole("link", { name: "Play from Series" }).getAttribute("href"),
    ).toBe("/videos/21?playlist=7&item=2");

    await user.click(screen.getByRole("link", { name: "Play from Video 13" }));
    expect(
      await screen.findByText(
        'Video page /videos/13?playlist=7&item=3 {"from":"/playlists/7","autoplay":true}',
      ),
    ).toBeDefined();
  });

  it("キーボードで項目を動かすと、上にある項目を付けて move を送り、読み直しても新しい順で出る", async () => {
    layoutRows();
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    const handle = screen.getByRole("button", { name: 'Reorder "Video 11"' });
    handle.focus();
    await user.keyboard("[Space]");
    await user.keyboard("[ArrowDown]");
    await user.keyboard("[Space]");

    await waitFor(() => expect(server.moves).toEqual([{ itemId: 1, afterItemId: 2 }]));
    expect(rowNames()).toEqual([
      "Play from Series",
      "Play from Video 11",
      "Play from Video 13",
    ]);

    cleanup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });
    expect(rowNames()).toEqual([
      "Play from Series",
      "Play from Video 11",
      "Play from Video 13",
    ]);
  });

  it("移動に失敗したら、サーバーが認めた順に戻してトーストを出す", async () => {
    layoutRows();
    server.moveStatus = 500;
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    screen.getByRole("button", { name: 'Reorder "Video 11"' }).focus();
    await user.keyboard("[Space]");
    await user.keyboard("[ArrowDown]");
    await user.keyboard("[Space]");

    expect(await screen.findByText(/Couldn't move the item/)).toBeDefined();
    expect(rowNames()).toEqual([
      "Play from Video 11",
      "Play from Series",
      "Play from Video 13",
    ]);
  });

  it("移動の 404 で読み直す間は次の移動を送らず、読んだ並びとサーバーの並びが合う", async () => {
    layoutRows();
    server.moveStatus = 404;
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    const release = hold();
    screen.getByRole("button", { name: 'Reorder "Video 11"' }).focus();
    await user.keyboard("[Space]");
    await user.keyboard("[ArrowDown]");
    await user.keyboard("[Space]");
    await waitFor(() => expect(server.getCalls).toBe(2));
    server.moveStatus = 204;

    // Move again before the refresh response arrives.
    screen.getByRole("button", { name: 'Reorder "Video 13"' }).focus();
    await user.keyboard("[Space]");
    await user.keyboard("[ArrowUp]");
    await user.keyboard("[ArrowUp]");
    await user.keyboard("[Space]");
    expect(server.moves).toEqual([{ itemId: 1, afterItemId: 2 }]);

    await act(async () => {
      release();
      await Promise.resolve();
    });
    await waitFor(() => expect(rowNames()).toEqual(serverNames()));
    expect(server.moves).toEqual([{ itemId: 1, afterItemId: 2 }]);
  });

  it("移動の 404 の後の読み直しに失敗したら、Retry で読み直す", async () => {
    layoutRows();
    server.moveStatus = 404;
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    server.getStatus = 500;
    screen.getByRole("button", { name: 'Reorder "Video 11"' }).focus();
    await user.keyboard("[Space]");
    await user.keyboard("[ArrowDown]");
    await user.keyboard("[Space]");

    expect(await screen.findByText("Couldn't load the playlist")).toBeDefined();
    expect(screen.queryByRole("table", { name: "Items" })).toBeNull();
    server.getStatus = 200;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByRole("table", { name: "Items" });
    expect(rowNames()).toEqual(serverNames());
  });

  it("項目を外すとその行がなくなり、番号を詰めて、次の行の × へフォーカスを移す", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    await user.click(
      screen.getByRole("button", { name: 'Remove "Series" from the playlist' }),
    );

    await waitFor(() =>
      expect(rowNames()).toEqual(["Play from Video 11", "Play from Video 13"]),
    );
    expect(server.removes).toEqual([2]);
    expect(screen.getByText("2 items · 2:00")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement?.getAttribute("aria-label")).toBe(
        'Remove "Video 13" from the playlist',
      ),
    );
  });

  it("最後の項目を外すと、追加の仕方を示し、「Play」を出さない", async () => {
    server.items = [{ id: 3, kind: "video", video: video(13) }];
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    await user.click(
      screen.getByRole("button", { name: 'Remove "Video 13" from the playlist' }),
    );

    expect(await screen.findByText("No items yet")).toBeDefined();
    expect(screen.getByRole("link", { name: "Open library" }).getAttribute("href")).toBe(
      "/",
    );
    expect(screen.queryByRole("link", { name: /from the top/ })).toBeNull();
    expect(screen.getByText("0 items")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 })),
    );
  });

  it("空のプレイリストには再生の操作がなく、項目の足し方を示す", async () => {
    server.items = [];
    renderPage();

    expect(await screen.findByText("No items yet")).toBeDefined();
    expect(
      screen.getByText(
        "Add videos and groups with Add to playlist on a card, on the video page, or in the selection bar.",
      ),
    ).toBeDefined();
    expect(screen.queryByRole("link", { name: /from the top/ })).toBeNull();
    expect(screen.getByRole("button", { name: "More" })).toBeDefined();
  });

  it("名前を変えるとトリムした名前を送り、見出しに出す。空の名前は送らない", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename…" }));
    const dialog = await screen.findByRole("dialog", { name: "Rename playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    expect((field as HTMLInputElement).value).toBe("Morning");
    expect(
      (within(dialog).getByRole("button", { name: "Save" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);

    await user.clear(field);
    await user.type(field, "   ");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(server.renames).toEqual([]);

    await user.clear(field);
    await user.type(field, "  Evening  ");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Evening" }),
    ).toBeDefined();
    expect(server.renames).toEqual([{ name: "Evening" }]);
    await waitFor(() => expect(document.title).toBe("Evening"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("削除は先に確認し、キャンセルでは何も変わらず、確定で /playlists へ移る", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    let dialog = await screen.findByRole("alertdialog", { name: 'Delete "Morning"?' });
    expect(
      within(dialog).getByText(
        "The playlist is deleted. Its videos and groups stay in the library.",
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(server.deletes).toBe(0);
    expect(rowNames()).toHaveLength(3);

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    dialog = await screen.findByRole("alertdialog", { name: 'Delete "Morning"?' });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Playlists screen")).toBeDefined();
    expect(server.deletes).toBe(1);
    expect(await screen.findByText('Deleted "Morning"')).toBeDefined();
  });

  it("別のプレイリストへ移ると、読み終えるまで前のプレイリストの名前と操作を出さない", async () => {
    renderPage();
    await screen.findByRole("table", { name: "Items" });
    expect(screen.getByRole("heading", { level: 1, name: "Morning" })).toBeDefined();

    act(() => navigateTo("/playlists/8"));

    await waitFor(() => expect(server.otherGets).toBe(1));
    expect(screen.queryByRole("heading", { level: 1, name: "Morning" })).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    expect(screen.queryByRole("table", { name: "Items" })).toBeNull();
  });

  it("別のプレイリストへ移った後に答えた削除は、移った先の画面を離れない", async () => {
    const user = userEvent.setup();
    let release: () => void = () => undefined;
    server.deleteHold = new Promise<void>((done) => {
      release = done;
    });
    renderPage();
    await screen.findByRole("table", { name: "Items" });
    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Morning"?' });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(server.deletes).toBe(1);

    act(() => navigateTo("/playlists/8"));
    await waitFor(() => expect(server.otherGets).toBe(1));
    await act(async () => {
      release();
      await Promise.resolve();
    });

    expect(await screen.findByText('Deleted "Morning"')).toBeDefined();
    expect(screen.queryByText("Playlists screen")).toBeNull();
  });

  it("404 ではプレイリストがなくなったことを示す", async () => {
    server.getStatus = 404;
    renderPage();

    expect(await screen.findByText("This playlist no longer exists")).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Back to playlists" }).getAttribute("href"),
    ).toBe("/playlists");
    expect(screen.getByRole("heading", { level: 1, name: "Playlists" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
  });

  it("読めない id はなくなったプレイリストとして示し、何も読まない", async () => {
    renderPage("/playlists/x");
    expect(await screen.findByText("This playlist no longer exists")).toBeDefined();
    expect(server.getCalls).toBe(0);
  });

  it("外す要求の 404 では、行を残したまま読み直す", async () => {
    server.removeStatus = 404;
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });
    expect(server.getCalls).toBe(1);

    server.items = server.items.filter((item) => item.id !== 2);
    await user.click(
      screen.getByRole("button", { name: 'Remove "Series" from the playlist' }),
    );

    await waitFor(() => expect(server.getCalls).toBe(2));
    await waitFor(() =>
      expect(rowNames()).toEqual(["Play from Video 11", "Play from Video 13"]),
    );
  });

  it("読み直しの間に外せた項目は、古い応答でも行に戻さない", async () => {
    server.removeStatus = 404;
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Items" });

    const release = hold();
    await user.click(
      screen.getByRole("button", { name: 'Remove "Series" from the playlist' }),
    );
    await waitFor(() => expect(server.getCalls).toBe(2));
    server.removeStatus = 204;
    await user.click(
      screen.getByRole("button", { name: 'Remove "Video 13" from the playlist' }),
    );
    await waitFor(() => expect(rowNames()).not.toContain("Play from Video 13"));

    await act(async () => {
      release();
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(rowNames()).toEqual(["Play from Video 11", "Play from Series"]),
    );
  });

  it("読み込みに失敗したら Retry で読み直す", async () => {
    server.getStatus = 500;
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("Couldn't load the playlist")).toBeDefined();
    server.getStatus = 200;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("table", { name: "Items" })).toBeDefined();
  });

  describe("スマートフォルダの項目", () => {
    it("グループの項目の行として名前・表紙・本数をサーバーの位置に出し、行はフォルダの開く動画を smartFolder 付きで開く", async () => {
      server.items = [server.items[0]!, folderItem(4, 4), server.items[2]!];
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("table", { name: "Items" });
      expect(rowNames()).toEqual([
        "Play from Video 11",
        "Play from タグ: 猫",
        "Play from Video 13",
      ]);
      expect(screen.getByText("3 items · 4:00")).toBeDefined();
      const rows = within(screen.getByRole("table", { name: "Items" })).getAllByRole(
        "row",
      );
      const folderRow = within(rows[1]!);
      expect(folderRow.getByText("4 videos")).toBeDefined();
      expect(folderRow.getByText("1 / 4")).toBeDefined();
      expect(rows[1]!.querySelector("img")?.getAttribute("src")).toBe(
        "/api/videos/31/thumbnail",
      );
      expect(rows[1]!.querySelector(".lucide-folder-search")).not.toBeNull();
      await user.click(screen.getByRole("link", { name: "Play from タグ: 猫" }));
      expect(
        await screen.findByText(
          'Video page /videos/31?playlist=7&item=4&smartFolder=9 {"from":"/playlists/7","autoplay":true}',
        ),
      ).toBeDefined();
    });

    it("グループの項目と同じく並べ替えて外せる", async () => {
      layoutRows();
      server.items = [server.items[0]!, folderItem(4, 4)];
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("table", { name: "Items" });
      const handle = screen.getByRole("button", { name: 'Reorder "タグ: 猫"' });
      handle.focus();
      await user.keyboard("[Space]");
      await user.keyboard("[ArrowUp]");
      await user.keyboard("[Space]");
      await waitFor(() =>
        expect(server.moves).toEqual([{ itemId: 4, afterItemId: null }]),
      );
      await user.click(
        screen.getByRole("button", { name: 'Remove "タグ: 猫" from the playlist' }),
      );
      await waitFor(() => expect(rowNames()).toEqual(["Play from Video 11"]));
      expect(server.removes).toEqual([4]);
    });

    it("動画の無いフォルダの項目は 0 videos の行でリンクにせず、「Play」は次の項目から始める", async () => {
      server.items = [folderItem(4, 0), server.items[2]!];
      renderPage();
      await screen.findByRole("table", { name: "Items" });
      expect(rowNames()).toEqual(["Play from Video 13"]);
      const rows = within(screen.getByRole("table", { name: "Items" })).getAllByRole(
        "row",
      );
      expect(within(rows[0]!).getByText("0 videos")).toBeDefined();
      expect(within(rows[0]!).getByText("タグ: 猫").className).toContain(
        "text-muted-foreground",
      );
      expect(within(rows[0]!).queryByLabelText("Watched")).toBeNull();
      expect(
        screen
          .getByRole("link", { name: 'Play "Morning" from the top' })
          .getAttribute("href"),
      ).toBe("/videos/13?playlist=7&item=3");
      expect(screen.getByText("2 items")).toBeDefined();
    });
  });

  it("画面の文言はカタログから出す", async () => {
    enablePseudoLocale();
    renderPage();
    await screen.findByRole("table", { name: /Items/ });
    // Duration is numeric formatting, not catalog copy.
    expectCatalogTextOnly(document.body, [
      "Morning",
      "Video 11",
      "Series",
      "Video 13",
      "1:00",
      "10:00",
    ]);
  });
});
