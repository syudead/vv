import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { OrganizableItem as LibraryItem } from "../api/libraryItems";
import { updateFavorites } from "../api/favorites";
import { updateVideoVisibility } from "../api/visibility";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { addTagInBulkEditor, editSummary } from "../testing/bulkEdit";
import LibraryPage from "./LibraryPage";

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
    id: 7,
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
  groupIds: number[];
  favorite: boolean;
}

interface Server {
  videos: Map<number, Video>;
  group: LibraryGroup | undefined;
  favoriteRequests: FavoritesBody[];
  libraryRequests: number;
  appliedGroups?: number;
  editSummaryRequests: number[][];
  editRequests: number[][];
  detailRequests: number[];
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
    editSummaryRequests: [],
    editRequests: [],
    detailRequests: [],
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
    if (url.pathname === "/api/tags") {
      return Promise.resolve(
        json({
          items: [{ id: 1, name: "旅行", synonyms: [], videoCount: 1, tentative: false }],
        }),
      );
    }
    if (url.pathname === "/api/video-edits/summary" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
      server.editSummaryRequests.push(body.videoIds);
      return Promise.resolve(json(editSummary(body.videoIds.length)));
    }
    if (url.pathname === "/api/video-edits" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
      server.editRequests.push(body.videoIds);
      return Promise.resolve(
        json({
          applied: body.videoIds.length,
          tags: [{ id: 1, name: "旅行", tentative: false, kind: null }],
        }),
      );
    }
    const detail = /^\/api\/videos\/(\d+)$/.exec(url.pathname);
    if (detail !== null && method === "GET") {
      const id = Number(detail[1]);
      server.detailRequests.push(id);
      return Promise.resolve(json(server.videos.get(id) ?? video(id)));
    }
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
    if (url.pathname === "/api/groups/7") {
      return Promise.resolve(
        server.group === undefined
          ? json({ code: "not_found" }, 404)
          : json({ group: server.group, trail: [], items: [] }),
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
      if (body.groupIds.length > 0 && server.group !== undefined) {
        server.group = { ...server.group, favorite: body.favorite };
      }
      const appliedGroups = server.appliedGroups ?? body.groupIds.length;
      server.appliedGroups = undefined;
      return Promise.resolve(
        json({
          appliedVideos: body.videoIds.length,
          appliedGroups,
          appliedSmartFolders: 0,
        }),
      );
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
  return server;
}

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

describe("ライブラリのお気に入りの付け外し", () => {
  it("送っている間は aria-disabled で、重ねて送らない", async () => {
    install([{ kind: "video", video: video(1) }]);
    const base = fetchMock.getMockImplementation();
    let answer: (() => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/favorites") {
        return new Promise<Response>((resolve) => {
          answer = () =>
            resolve(json({ appliedVideos: 1, appliedGroups: 0, appliedSmartFolders: 0 }));
        });
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    await user.click(toggle);
    expect(toggle.getAttribute("aria-disabled")).toBe("true");
    await user.click(toggle);
    const puts = fetchMock.mock.calls.filter(
      ([input]) => String(input) === "/api/favorites",
    );
    expect(puts).toHaveLength(1);
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
    useListView();
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
    expect(toggle.className).toContain("text-muted-foreground");

    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toEqual([
      { videoIds: [1], groupIds: [], favorite: true },
    ]);
    expect(toggle.className).toContain("text-favorite");
    expect(toggle.className).toContain("size-6");
    expect(toggle.querySelector("svg")?.getAttribute("class")).not.toContain(
      "drop-shadow-mark",
    );
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("false"));
  });

  it("グループの行は groupIds: [id] を送り、取り直して印が変わり、メンバーには影響しない", async () => {
    const server = install(
      [
        { kind: "video", video: video(101, { title: "ep01" }) },
        { kind: "group", group: seriesGroup() },
      ],
      seriesGroup(),
    );
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite group "series"' });
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toEqual([
      { videoIds: [], groupIds: [7], favorite: true },
    ]);
    const groupFetches = fetchMock.mock.calls.filter(
      ([input]) =>
        new URL(String(input), "http://localhost").pathname === "/api/groups/7",
    );
    expect(groupFetches).toHaveLength(1);
    expect(pressed(screen.getByRole("button", { name: 'Favorite "ep01"' }))).toBe(
      "false",
    );
  });

  it("フォルダの無い（手で作った）グループの行も、その id で付け外しできる", async () => {
    const { folder: _folder, ...handMade } = seriesGroup({ id: 8, name: "mix" });
    const server = install([{ kind: "group", group: handMade }], handMade);
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite group "mix"' });
    await user.click(toggle);
    await waitFor(() => expect(server.favoriteRequests).toHaveLength(1));
    expect(server.favoriteRequests).toEqual([
      { videoIds: [], groupIds: [8], favorite: true },
    ]);
    expect(
      screen.getByRole("button", { name: 'Add group "mix" to a playlist' }),
    ).toBeDefined();
  });

  it("appliedGroups が 0 で、取り直しが 404 なら、その行を一覧から外す", async () => {
    const server = install(
      [
        { kind: "video", video: video(1) },
        { kind: "group", group: seriesGroup() },
      ],
      seriesGroup(),
    );
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    const toggle = await screen.findByRole("button", { name: 'Favorite group "series"' });
    server.group = undefined;
    server.appliedGroups = 0;
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
    useListView();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
    expect(screen.getByText("1 video selected")).toBeDefined();

    const toggle = screen.getByRole("button", { name: 'Favorite "動画 1"' });
    await user.click(toggle);
    await waitFor(() => expect(pressed(toggle)).toBe("true"));
    expect(server.favoriteRequests).toHaveLength(1);
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
    useListView();
    renderLibrary();
    await user.click(await screen.findByRole("link", { name: "動画 1" }));
    await screen.findByText("再生画面");

    await act(async () => {
      await updateFavorites([1], [7], true);
    });
    await user.click(screen.getByRole("button", { name: "一覧へ戻る" }));

    const toggle = await screen.findByRole("button", { name: 'Favorite "動画 1"' });
    expect(pressed(toggle)).toBe("true");
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
        return new Promise<Response>((resolve) => {
          answerPage = () =>
            resolve(json({ items: [{ kind: "group", group: seriesGroup() }], total: 1 }));
        });
      }
      return base!(input, init);
    });
    renderLibrary();
    await waitFor(() => expect(answerPage).toBeDefined());
    server.group = undefined;
    server.appliedGroups = 0;
    await act(async () => {
      await updateFavorites([], [7], true);
    });
    act(() => answerPage?.());
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([input]) =>
            new URL(String(input), "http://localhost").pathname === "/api/groups/7",
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
          return base!(input, init).then(() =>
            json({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 0 }),
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
      useListView();
      renderLibrary();
      await screen.findByRole("button", { name: 'Favorite "動画 1"' });
      await partialFavorite();
      await act(async () => {
        await updateVideoVisibility([1], true);
      });
      const toggle = await openAndReturn(user);
      await waitFor(() => expect(server.libraryRequests).toBe(2));
      await waitFor(() => expect(pressed(toggle)).toBe("true"));
    });

    it("全件に反映されたお気に入りの結果が届けば確かになり、控えから戻す", async () => {
      const server = installPartial();
      const user = userEvent.setup();
      useListView();
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

describe("選択バーの一括のお気に入り", () => {
  const seriesGroupId = 7;

  async function favoriteSelection(user: ReturnType<typeof userEvent.setup>, add = true) {
    await user.click(screen.getByRole("button", { name: "Favorite" }));
    const items = await screen.findAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "Add to favorites",
      "Remove from favorites",
    ]);
    await user.click(
      screen.getByRole("menuitem", {
        name: add ? "Add to favorites" : "Remove from favorites",
      }),
    );
  }

  it("動画 2 本とグループ 1 つを付けると、videoIds は 2 本・groupIds は 1 つで、3 枚の印が付く（受け入れ条件 7）", async () => {
    const group = seriesGroup();
    const server = install(
      [
        { kind: "video", video: video(1) },
        { kind: "video", video: video(2) },
        { kind: "group", group },
      ],
      group,
    );
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "series"' }));
    expect(screen.getByText("5 videos selected")).toBeDefined();

    const bar = screen.getByRole("region", { name: "Selection actions" });
    const names = within(bar)
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));
    expect(
      names.slice(names.indexOf("Edit"), names.indexOf("Bundle as versions") + 1),
    ).toEqual([
      "Edit",
      "Favorite",
      "Add to playlist",
      "New group",
      "Add to group",
      "Bundle as versions",
    ]);

    await favoriteSelection(user);
    expect(await screen.findByText("Added 3 items to favorites")).toBeDefined();
    expect(server.favoriteRequests).toEqual([
      { videoIds: [1, 2], groupIds: [seriesGroupId], favorite: true },
    ]);
    await waitFor(() =>
      expect(
        pressed(screen.getByRole("button", { name: 'Favorite group "series"' })),
      ).toBe("true"),
    );
    expect(pressed(screen.getByRole("button", { name: 'Favorite "動画 1"' }))).toBe(
      "true",
    );
    expect(pressed(screen.getByRole("button", { name: 'Favorite "動画 2"' }))).toBe(
      "true",
    );
    expect(screen.getByText("5 videos selected")).toBeDefined();
  });

  it("グループのメンバーを 1 本外してから付けると、残ったメンバーを動画として送り、グループは送らない", async () => {
    const group = seriesGroup();
    const server = install(
      [
        { kind: "group", group },
        { kind: "video", video: video(101) },
      ],
      group,
    );
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 101"' }));
    expect(screen.getByText("2 videos selected")).toBeDefined();
    await favoriteSelection(user);
    expect(await screen.findByText("Added 2 items to favorites")).toBeDefined();
    expect(server.favoriteRequests).toEqual([
      { videoIds: [102, 103], groupIds: [], favorite: true },
    ]);
  });

  it("既にお気に入りのものを含めても誤りにならず、トーストは appliedVideos + appliedGroups を数える（Edge Case）", async () => {
    const group = seriesGroup({ favorite: true });
    install(
      [
        { kind: "video", video: video(1, { favorite: true }) },
        { kind: "group", group },
      ],
      group,
    );
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "series"' }));
    await favoriteSelection(user);
    expect(await screen.findByText("Added 2 items to favorites")).toBeDefined();
    expect(screen.queryByText(/^Couldn't change the favorites/)).toBeNull();
  });

  it("1 件ならトーストの単位は item で、外すときも同じ分け方で送る", async () => {
    const group = seriesGroup({ favorite: true });
    const server = install(
      [
        { kind: "video", video: video(1, { favorite: true }) },
        { kind: "group", group },
      ],
      group,
    );
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    await favoriteSelection(user, false);
    expect(await screen.findByText("Removed 1 item from favorites")).toBeDefined();
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
    await favoriteSelection(user, false);
    expect(await screen.findByText("Removed 2 items from favorites")).toBeDefined();
    expect(server.favoriteRequests).toEqual([
      { videoIds: [], groupIds: [seriesGroupId], favorite: false },
      { videoIds: [1], groupIds: [seriesGroupId], favorite: false },
    ]);
    await waitFor(() =>
      expect(
        pressed(screen.getByRole("button", { name: 'Favorite group "series"' })),
      ).toBe("false"),
    );
    expect(pressed(screen.getByRole("button", { name: 'Favorite "動画 1"' }))).toBe(
      "false",
    );
  });

  it("失敗したらトーストで理由を伝え、選択を残す", async () => {
    install([{ kind: "video", video: video(1) }]);
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) =>
      String(input) === "/api/favorites"
        ? Promise.resolve(json({ code: "internal", message: "boom" }, 500))
        : base!(input, init),
    );
    const user = userEvent.setup();
    useListView();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await favoriteSelection(user);
    expect(await screen.findByText(/^Couldn't change the favorites: /)).toBeDefined();
    expect(screen.getByText("1 video selected")).toBeDefined();
    expect(pressed(screen.getByRole("button", { name: 'Favorite "動画 1"' }))).toBe(
      "false",
    );
  });

  it("メンバーが 20,000 本を超えるグループ 1 つは、一括編集は押せず、お気に入りは groupIds 1 つとして押せる", async () => {
    const huge = Array.from({ length: 20_001 }, (_, index) => 100_000 + index);
    const group = seriesGroup({ videoIds: huge, videoCount: huge.length });
    const server = install([{ kind: "group", group }], group);
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    expect(screen.getByText("20,001 videos selected")).toBeDefined();
    expect(
      (screen.getByRole("button", { name: "Edit" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    const favorite = screen.getByRole("button", {
      name: "Favorite",
    }) as HTMLButtonElement;
    expect(favorite.disabled).toBe(false);
    expect(favorite.getAttribute("aria-describedby")).toBeNull();
    await favoriteSelection(user);
    expect(await screen.findByText("Added 1 item to favorites")).toBeDefined();
    expect(server.favoriteRequests).toEqual([
      { videoIds: [], groupIds: [seriesGroupId], favorite: true },
    ]);
  });

  it("送る数が 20,000 を超えると、一括編集と同じ理由で押せない", async () => {
    const huge = Array.from({ length: 20_001 }, (_, index) => 100_000 + index);
    const group = seriesGroup({ videoIds: huge, videoCount: huge.length });
    install(
      [
        { kind: "group", group },
        { kind: "video", video: video(100_000) },
      ],
      group,
    );
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 100000"' }));
    const favorite = screen.getByRole("button", {
      name: "Favorite",
    }) as HTMLButtonElement;
    expect(favorite.disabled).toBe(false);
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 100000"' }));
    expect(favorite.disabled).toBe(true);
    expect(favorite.title).toBe("Select between 1 and 20,000 videos.");
    const describedBy = favorite.getAttribute("aria-describedby");
    expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
      "Select between 1 and 20,000 videos.",
    );
  });

  it("一括編集と束ねる操作は今までどおりメンバーを含む動画の id を送る", async () => {
    const group = seriesGroup();
    const server = install(
      [
        { kind: "video", video: video(1) },
        { kind: "group", group },
      ],
      group,
    );
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "series"' }));
    const expected = [1, 101, 102, 103];

    await addTagInBulkEditor(user, "旅行");
    expect(await screen.findByText("Updated 4 videos")).toBeDefined();
    expect(server.editSummaryRequests).toEqual([expected]);
    expect(server.editRequests).toEqual([expected]);

    await user.click(screen.getByRole("button", { name: "Bundle as versions" }));
    await screen.findByRole("dialog");
    await waitFor(() =>
      expect([...server.detailRequests].sort((a, b) => a - b)).toEqual(expected),
    );
    expect(server.favoriteRequests).toEqual([]);
  });
});
