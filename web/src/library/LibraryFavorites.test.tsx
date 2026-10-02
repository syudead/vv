import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { updateVideoVisibility } from "../api/visibility";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import LibraryPage from "./LibraryPage";

/**
 * ライブラリのカードと行のお気に入りの付け外し（specs/035-favorites/ui-design.md「Card」、
 * issue 660）。
 */

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024 * 1024 * id,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    durationMs: 60_000 * id,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "pending",
    tags: [],
    favorite: false,
    ...extra,
  };
}

const memberIds = [101, 102, 103];

function seriesGroup(extra: Partial<LibraryGroup> = {}): LibraryGroup {
  return {
    folder: { rootId: 3, path: "series" },
    name: "series",
    videoCount: 3,
    watchedCount: 0,
    watchState: "unwatched",
    durationMs: 3 * 60_000,
    sizeBytes: 3 * 1024 * 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: memberIds.map((id) => ({
      videoId: id,
      thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    })),
    openVideoId: 101,
    videoIds: memberIds,
    tags: [],
    favorite: false,
    ...extra,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

interface FavoritesBody {
  videoIds: number[];
  folders: { rootId: number; path: string }[];
  favorite: boolean;
}

interface Server {
  videos: Map<number, Video>;
  group: LibraryGroup | undefined;
  favoriteRequests: FavoritesBody[];
  libraryRequests: number;
  /** 次の PUT /api/favorites の appliedFolders を上書きする。 */
  appliedFolders?: number;
}

const fetchMock = vi.fn<typeof fetch>();

function install(items: LibraryItem[], group?: LibraryGroup): Server {
  const server: Server = {
    videos: new Map(
      items.flatMap((item) =>
        item.kind === "video" ? [[item.video.id, item.video]] : [],
      ),
    ),
    group,
    favoriteRequests: [],
    libraryRequests: 0,
  };
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
    if (url.pathname === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/library") {
      server.libraryRequests += 1;
      return Promise.resolve(
        json({
          items: items.map((item) =>
            item.kind === "video"
              ? { kind: "video", video: server.videos.get(item.video.id) }
              : { kind: "group", group: server.group },
          ),
          total: items.length,
        }),
      );
    }
    if (url.pathname === "/api/folders/3/group") {
      return Promise.resolve(
        server.group === undefined
          ? json({ code: "not_found" }, 404)
          : json(server.group),
      );
    }
    if (url.pathname === "/api/favorites" && method === "PUT") {
      const body = JSON.parse(String(init?.body)) as FavoritesBody;
      server.favoriteRequests.push(body);
      for (const id of body.videoIds) {
        const current = server.videos.get(id);
        if (current !== undefined) {
          server.videos.set(id, { ...current, favorite: body.favorite });
        }
      }
      if (body.folders.length > 0 && server.group !== undefined) {
        server.group = { ...server.group, favorite: body.favorite };
      }
      const appliedFolders = server.appliedFolders ?? body.folders.length;
      server.appliedFolders = undefined;
      return Promise.resolve(
        json({ appliedVideos: body.videoIds.length, appliedFolders }),
      );
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
  return server;
}

/** PlayerStub は再生画面の代わりに、元の一覧へ戻る操作を置く。 */
function PlayerStub() {
  const location = useLocation();
  const navigate = useNavigate();
  const from = (location.state as { from?: string } | null)?.from ?? "/";
  return (
    <>
      <p>再生画面</p>
      <button type="button" onClick={() => void navigate(from)}>
        一覧へ戻る
      </button>
    </>
  );
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
                <Route path="/videos/:id" element={<PlayerStub />} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function useListView() {
  localStorage.setItem(
    "vv.view.v2",
    JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
  );
}

function pressed(element: HTMLElement): string | null {
  return element.getAttribute("aria-pressed");
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
});

describe("ライブラリのお気に入りの付け外し（specs/035-favorites/ui-design.md「Card」）", () => {
  it("動画のカードで押すと videoIds: [id] を送り、応答のあと同じ場所の印が付き、もう一度押すと外れる", async () => {
    const server = install([
      { kind: "video", video: video(1) },
      { kind: "video", video: video(2) },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    expect(pressed(toggle)).toBe("false");
    // オフは hover・フォーカス・hover:none の端末でだけ見える（チェックと同じ条件）。
    expect(toggle.className).toContain("opacity-0");
    expect(toggle.className).toContain("[@media(hover:none)]:opacity-100");
    // リンクの外、同じ article の中（チェック → リンク → 付け外し）。
    const card = toggle.closest("article") as HTMLElement;
    expect(toggle.closest("a")).toBeNull();
    const link = within(card).getByRole("link", { name: "動画 1" });
    expect(
      link.compareDocumentPosition(toggle) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toEqual([
      { videoIds: [1], folders: [], favorite: true },
    ]);
    expect(toggle.className).toContain("text-link");
    expect(toggle.className).not.toContain("opacity-0");
    // 押しても再生画面は開かない。
    expect(screen.queryByText("再生画面")).toBeNull();
    // ほかのカードは変わらない。
    expect(pressed(screen.getByRole("button", { name: 'Favorite "動画 2"' }))).toBe(
      "false",
    );

    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("false"));
    expect(server.favoriteRequests[1]).toEqual({
      videoIds: [1],
      folders: [],
      favorite: false,
    });
    // 一覧は読み直さない（その場で差し替える）。
    expect(server.libraryRequests).toBe(1);
  });

  it("送っている間は aria-disabled で、重ねて送らない", async () => {
    install([{ kind: "video", video: video(1) }]);
    const base = fetchMock.getMockImplementation();
    let answer: (() => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/favorites") {
        return new Promise<Response>((resolve) => {
          answer = () => resolve(json({ appliedVideos: 1, appliedFolders: 0 }));
        });
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    await user.click(toggle);
    expect(toggle.getAttribute("aria-disabled")).toBe("true");
    await user.click(toggle);
    const puts = fetchMock.mock.calls.filter(
      ([input]) => String(input) === "/api/favorites",
    );
    expect(puts).toHaveLength(1);
    // 応答の前は印を変えない。
    expect(pressed(toggle)).toBe("false");
    act(() => answer?.());
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(toggle.getAttribute("aria-disabled")).toBeNull();
  });

  it("失敗したらトーストを出し、印は変えない", async () => {
    install([{ kind: "video", video: video(1) }]);
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) =>
      String(input) === "/api/favorites"
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : base!(input, init),
    );
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    await user.click(toggle);
    expect(await screen.findByText(/^Couldn't change the favorite: /)).toBeDefined();
    expect(pressed(toggle)).toBe("false");
  });

  it("リスト表示の行でも、題名の直後の列で同じに付け外しできる", async () => {
    useListView();
    const server = install([{ kind: "video", video: video(1) }]);
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    const cell = toggle.closest("td") as HTMLElement;
    expect(cell.className).toContain("w-8");
    const titleCell = cell.previousElementSibling as HTMLElement;
    expect(within(titleCell).getByRole("link", { name: "動画 1" })).toBeDefined();
    expect(toggle.className).toContain("text-fg-muted");

    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toEqual([
      { videoIds: [1], folders: [], favorite: true },
    ]);
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("false"));
  });

  it("グループのカードは folders: [folder] を送り、取り直して印が変わり、メンバーには影響しない", async () => {
    const server = install(
      [
        { kind: "video", video: video(101, { title: "ep01" }) },
        { kind: "group", group: seriesGroup() },
      ],
      seriesGroup(),
    );
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite group "series"' });
    expect(toggle.closest("div")?.className).toContain("z-30");
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toEqual([
      { videoIds: [], folders: [{ rootId: 3, path: "series" }], favorite: true },
    ]);
    const groupFetches = fetchMock.mock.calls.filter(
      ([input]) =>
        new URL(String(input), "http://localhost").pathname === "/api/folders/3/group",
    );
    expect(groupFetches).toHaveLength(1);
    expect(pressed(screen.getByRole("button", { name: 'Favorite "ep01"' }))).toBe(
      "false",
    );
  });

  it("リスト表示のグループの行でも付け外しできる", async () => {
    useListView();
    install([{ kind: "group", group: seriesGroup() }], seriesGroup());
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite group "series"' });
    expect(toggle.closest("td")?.className).toContain("w-8");
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
  });

  it("appliedFolders が 0 で、取り直しが 404 なら、そのカードを一覧から外す", async () => {
    const server = install(
      [
        { kind: "video", video: video(1) },
        { kind: "group", group: seriesGroup() },
      ],
      seriesGroup(),
    );
    const user = userEvent.setup();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite group "series"' });
    // もうグループではない。
    server.group = undefined;
    server.appliedFolders = 0;
    await user.click(toggle);
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: 'Favorite group "series"' }),
      ).toBeNull(),
    );
    expect(screen.getByRole("link", { name: "動画 1" })).toBeDefined();
  });

  it("押してもリンクと選択は動かず、選択モードでも付け外しできる", async () => {
    const server = install([
      { kind: "video", video: video(1) },
      { kind: "video", video: video(2) },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
    expect(screen.getByText("1 video selected")).toBeDefined();

    const toggle = screen.getByRole("button", { name: 'Favorite "動画 1"' });
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toHaveLength(1);
    // 選択は変わらず、動画 1 は選ばれない。
    expect(screen.getByText("1 video selected")).toBeDefined();
    expect(
      screen
        .getByRole("checkbox", { name: 'Select "動画 1"' })
        .getAttribute("aria-checked"),
    ).toBe("false");
    expect(screen.queryByText("再生画面")).toBeNull();
  });

  it("ゲストのカードと行には印も付け外しも無い", async () => {
    install([{ kind: "video", video: video(1, { public: true, favorite: undefined }) }]);
    renderLibrary("guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: /^Favorite/ })).toBeNull();
    cleanup();

    useListView();
    renderLibrary("guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: /^Favorite/ })).toBeNull();
    const row = screen.getByRole("link", { name: "動画 1" }).closest("tr") as HTMLElement;
    expect(row.querySelector("td.w-8")).toBeNull();
  });

  it("一覧の控えを復元したときも、お気に入りの結果が反映されている", async () => {
    const server = install(
      [
        { kind: "video", video: video(1) },
        { kind: "group", group: seriesGroup() },
      ],
      seriesGroup(),
    );
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await screen.findByRole("link", { name: "動画 1" }));
    await screen.findByText("再生画面");

    // 再生画面にいる間に付ける（再生画面の付け外しと同じ経路）。
    await act(async () => {
      await updateFavorites([1], [{ rootId: 3, path: "series" }], true);
    });
    await user.click(screen.getByRole("button", { name: "一覧へ戻る" }));

    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    expect(pressed(toggle)).toBe("true");
    // 一覧は読み直さずに控えから戻し、グループは取り直す。
    expect(server.libraryRequests).toBe(1);
    await waitFor(() =>
      expect(
        pressed(screen.getByRole("button", { name: 'Favorite group "series"' })),
      ).toBe("true"),
    );
  });
  it("ページの取得中に付け外したグループが、もうグループでなければ、届いたページから外す", async () => {
    const server = install([{ kind: "group", group: seriesGroup() }], seriesGroup());
    const base = fetchMock.getMockImplementation();
    let answerPage: (() => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      if (new URL(String(input), "http://localhost").pathname === "/api/library") {
        // 付け外しの前に読まれた、古いグループを含むページ。
        return new Promise<Response>((resolve) => {
          answerPage = () =>
            resolve(json({ items: [{ kind: "group", group: seriesGroup() }], total: 1 }));
        });
      }
      return base!(input, init);
    });
    renderLibrary();
    await waitFor(() => expect(answerPage).toBeDefined());
    // ページが届く前に、もうグループでないフォルダを付け外す（appliedFolders: 0）。
    server.group = undefined;
    server.appliedFolders = 0;
    await act(async () => {
      await updateFavorites([], [{ rootId: 3, path: "series" }], true);
    });
    act(() => answerPage?.());
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([input]) =>
            new URL(String(input), "http://localhost").pathname ===
            "/api/folders/3/group",
        ),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: 'Favorite group "series"' }),
      ).toBeNull(),
    );
  });

  describe("一部しか反映されず取り直しに失敗した動画（確かでない理由は別々に持つ）", () => {
    function installPartial() {
      const server = install([{ kind: "video", video: video(1) }]);
      const base = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        const path = new URL(String(input), "http://localhost").pathname;
        if (path === "/api/videos/1") {
          return Promise.resolve(json({ code: "internal", message: "boom" }, 500));
        }
        if (path === "/api/video-visibility") {
          return Promise.resolve(json({ applied: 1 }));
        }
        if (path === "/api/favorites" && server.favoriteRequests.length === 0) {
          // 最初の付け外しは反映の数が足りない（サーバーでは付いている）。
          return base!(input, init).then(() =>
            json({ appliedVideos: 0, appliedFolders: 0 }),
          );
        }
        return base!(input, init);
      });
      return server;
    }

    async function partialFavorite() {
      await act(async () => {
        await updateFavorites([1], [], true);
      });
      await waitFor(() =>
        expect(
          fetchMock.mock.calls.some(
            ([input]) =>
              new URL(String(input), "http://localhost").pathname === "/api/videos/1",
          ),
        ).toBe(true),
      );
    }

    async function openAndReturn(user: ReturnType<typeof userEvent.setup>) {
      await user.click(screen.getByRole("link", { name: "動画 1" }));
      await screen.findByText("再生画面");
      await user.click(screen.getByRole("button", { name: "一覧へ戻る" }));
      return screen.findByRole("button", { name: 'Favorite "動画 1"' });
    }

    it("全件に反映された公開の切り替えでは、お気に入りの印は確かにならず、古い控えを戻さない", async () => {
      const server = installPartial();
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("button", { name: 'Favorite "動画 1"' });
      await partialFavorite();
      await act(async () => {
        await updateVideoVisibility([1], true);
      });
      const toggle = await openAndReturn(user);
      // 控えは取られず、一覧を読み直してサーバーの印を出す。
      await waitFor(() => expect(server.libraryRequests).toBe(2));
      await waitFor(() => expect(pressed(toggle)).toBe("true"));
    });

    it("全件に反映されたお気に入りの結果が届けば確かになり、控えから戻す", async () => {
      const server = installPartial();
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("button", { name: 'Favorite "動画 1"' });
      await partialFavorite();
      await act(async () => {
        await updateFavorites([1], [], true);
      });
      const toggle = await openAndReturn(user);
      expect(pressed(toggle)).toBe("true");
      expect(server.libraryRequests).toBe(1);
    });
  });
});
