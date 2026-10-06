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
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video, VideoPage } from "../api/client";
import { FakeEventSource, installFakeEventSource } from "../api/fakeEventSource";
import { videoItem } from "../api/libraryItems";
import { clearListSnapshot } from "../api/listSnapshot";
import { nextProgressSequence, recordSavedProgress } from "../api/progressEvents";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { formatBytes, formatDuration } from "../lib/format";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { resultCountText } from "../videoList/listSummary";
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
    tags: [],
    ...extra,
    previewState: extra.previewState ?? "pending",
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** LocationProbe は今の URL を見せ、履歴を1つ戻る操作を置く（戻る/進むの確認）。 */
function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <span data-testid="location">{location.search}</span>
      <button type="button" onClick={() => void navigate(-1)}>
        テストで戻る
      </button>
    </>
  );
}

function listRequests(fetchMock: ReturnType<typeof vi.fn<typeof fetch>>): URL[] {
  return fetchMock.mock.calls
    .map((call) => new URL(String(call[0]), "http://localhost"))
    .filter((url) => url.pathname === "/api/library");
}

/**
 * asLibraryResponse は、動画だけの一覧（VideoPage の形）で書いたテストの応答を
 * GET /api/library の形（items が LibraryItem）に包む。グループを含む応答は
 * 最初から LibraryItem で書くので、`kind` を持つ項目はそのまま通す。
 */
async function asLibraryResponse(response: Response): Promise<Response> {
  if (!response.ok) return response;
  const body = (await response.clone().json()) as { items?: unknown[] };
  if (!Array.isArray(body.items)) return response;
  return json(
    {
      ...body,
      items: body.items.map((item) =>
        typeof item === "object" && item !== null && "kind" in item
          ? item
          : { kind: "video", video: item },
      ),
    },
    response.status,
  );
}

/** libraryFetch は fetchMock の GET /api/library の応答を asLibraryResponse で包む。 */
function libraryFetch(fetchMock: ReturnType<typeof vi.fn<typeof fetch>>): typeof fetch {
  return (input, init) => {
    const result = fetchMock(input, init);
    const path = new URL(String(input), "http://localhost").pathname;
    return path === "/api/library" ? result.then(asLibraryResponse) : result;
  };
}

function renderLibrary(initial = "/", audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route
                  path="/"
                  element={
                    <>
                      <LibraryPage />
                      <LocationProbe />
                    </>
                  }
                />
                <Route path="/videos/:id" element={<p>再生画面</p>} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("resultCountText", () => {
  it("サーバーの全件数だけを表示する", () => {
    expect(resultCountText(59)).toBe("59 videos");
    expect(resultCountText(1_234)).toBe("1,234 videos");
  });
});

describe("LibraryPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    __resetTagsForTest();
    clearListSnapshot();
    vi.stubGlobal("fetch", libraryFetch(fetchMock));
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      const page: VideoPage = {
        items: [
          video(1),
          video(2, { progress: { positionMs: 30_000, completed: false, updatedAt: "" } }),
          video(3, { progress: { positionMs: 0, completed: true, updatedAt: "" } }),
        ],
        total: 3,
      };
      return Promise.resolve(json(page));
    });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("一覧と件数だけを出す", async () => {
    renderLibrary();
    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
    const status = screen.getByRole("status");
    expect(status.textContent).toBe("3 items");
    expect(status.classList.contains("sr-only")).toBe(false);
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("25");
  });

  it("検索語は URL から読み、結果件数だけを出す", async () => {
    renderLibrary("/?q=abc");
    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toBe("3 items");
    });
    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => String(call[0]));
      expect(calls.some((url) => url.includes("query=abc"))).toBe(true);
    });
  });

  it("狭い画面向けメニューから省略された表示操作を使える", async () => {
    const user = userEvent.setup();
    renderLibrary();

    await user.click(screen.getByRole("button", { name: "View and sort" }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("radio", { name: "Title" })).toBeDefined();
    expect(
      within(dialog).getByRole("radiogroup", { name: "View (compact)" }),
    ).toBeDefined();
    expect(within(dialog).getByRole("slider", { name: "Card size" })).toBeDefined();
  });

  it("空なら取り込みを促す", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input).startsWith("/api/scans/current")
          ? json({}, 404)
          : String(input) === "/api/media-folders"
            ? json([{}])
            : json({ items: [], total: 0 } satisfies VideoPage),
      ),
    );
    renderLibrary();
    expect(await screen.findByText("No videos yet")).toBeDefined();
    // 押しても取り込みを始めず、開始の場所である設定の「Scan status」へ移る。
    expect(screen.getByRole("link", { name: "Scan" }).getAttribute("href")).toBe(
      "/settings#scan-status",
    );
  });

  it("失敗なら再試行を出し、再試行中は読み込み表示へ戻す", async () => {
    let attempts = 0;
    let resolveRetry: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      attempts++;
      if (attempts === 1) {
        return Promise.resolve(json({ code: "internal", message: "broken" }, 500));
      }
      return new Promise<Response>((resolve) => {
        resolveRetry = resolve;
      });
    });
    const user = userEvent.setup();
    renderLibrary();
    expect(await screen.findByText("Something went wrong on the server.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Retry" })).toBeDefined();
    expect(screen.queryByRole("status")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(resolveRetry).toBeDefined());
    expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
      "Loading…",
    );
    expect(screen.queryByText("Something went wrong on the server.")).toBeNull();

    await act(async () => {
      resolveRetry?.(json({ items: [video(1)], total: 1 } satisfies VideoPage));
    });
    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
  });

  it("条件変更後の取得に失敗したら前の件数を残さない", async () => {
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/library" && url.searchParams.get("query") === "broken") {
        return Promise.resolve(json({ code: "internal", message: "broken" }, 500));
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    renderLibrary();
    expect((await screen.findByRole("status")).textContent).toBe("3 items");

    await user.type(screen.getByRole("searchbox", { name: "Search videos" }), "broken");

    expect(await screen.findByText("Something went wrong on the server.")).toBeDefined();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("視聴状態はサーバーに送り、残りのページを読みに行かず、件数はサーバーの total を出す", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      const watch = new URL(url, "http://localhost").searchParams.get("watch");
      return Promise.resolve(
        json({
          items: watch === "unwatched" ? [video(1)] : [video(1), video(2)],
          total: watch === "unwatched" ? 250 : 500,
          nextCursor: "cursor-1",
        } satisfies VideoPage),
      );
    });
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 2" });

    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("radio", { name: "Unwatched" }));

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "動画 2" })).toBeNull(),
    );
    expect(screen.getByRole("status").textContent).toBe("250 items");
    const requests = listRequests(fetchMock);
    expect(requests.at(-1)?.searchParams.get("watch")).toBe("unwatched");
    // 選んだ瞬間に続きのページを読みに行かない。
    expect(requests.filter((url) => url.searchParams.has("cursor"))).toHaveLength(0);
    expect(screen.getByTestId("location").textContent).toBe(
      "?watch=unwatched&sort=addedDesc",
    );
    expect(screen.getByRole("button", { name: "Filter (1 applied)" })).toBeDefined();
  });

  it("URL の条件をそのまま一覧の要求に載せる", async () => {
    renderLibrary("/?q=%E4%BA%AC%E9%83%BD&watch=inProgress&playable=1&sort=durationAsc");
    await screen.findByRole("link", { name: "動画 1" });
    const request = listRequests(fetchMock).at(-1);
    expect(request?.searchParams.get("query")).toBe("京都");
    expect(request?.searchParams.get("watch")).toBe("inProgress");
    expect(request?.searchParams.get("playable")).toBe("true");
    expect(request?.searchParams.get("sort")).toBe("durationAsc");
    expect(screen.getByRole("button", { name: "Sort by: Length" })).toBeDefined();
    expect(
      screen.getByRole("button", {
        name: "Ascending (shortest first). Press for descending",
      }),
    ).toBeDefined();
  });

  it("random で seed が無ければ作って URL に書き足し、履歴は増やさない", async () => {
    renderLibrary("/?sort=random");
    await screen.findByRole("link", { name: "動画 1" });
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toMatch(
        /^\?sort=random&seed=\d+$/,
      ),
    );
    const seed = new URLSearchParams(
      screen.getByTestId("location").textContent ?? "",
    ).get("seed");
    const requests = listRequests(fetchMock);
    expect(requests.every((url) => url.searchParams.get("seed") === seed)).toBe(true);
    expect(screen.getByRole("button", { name: "Shuffle" })).toBeDefined();
  });

  it("並べ直すは新しい seed で読み直し、戻るで前の並びに戻る", async () => {
    const user = userEvent.setup();
    renderLibrary("/?sort=random&seed=7");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getAllByRole("button", { name: "Shuffle" })[0]!);
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).not.toBe("?sort=random&seed=7"),
    );
    const seed = new URLSearchParams(
      screen.getByTestId("location").textContent ?? "",
    ).get("seed");
    expect(seed).not.toBe("7");
    await waitFor(() =>
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("seed")).toBe(seed),
    );

    await user.click(screen.getByRole("button", { name: "テストで戻る" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=random&seed=7");
  });

  it("向きの切り替えは同じ種類の逆向きにし、履歴を1つ増やす", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(
      screen.getByRole("button", {
        name: "Descending (newest first). Press for ascending",
      }),
    );
    expect(screen.getByTestId("location").textContent).toBe("?sort=addedAsc");
    await waitFor(() =>
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe("addedAsc"),
    );
    expect(
      screen.getByRole("button", {
        name: "Ascending (oldest first). Press for descending",
      }),
    ).toBeDefined();

    // 最初の URL（sort なし）も戻り先として並び順を持つので、保存した並び順が
    // 変わっても戻ると前の並びになる。
    await user.click(screen.getByRole("button", { name: "テストで戻る" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=addedDesc");
    expect(
      screen.getByRole("button", {
        name: "Descending (newest first). Press for ascending",
      }),
    ).toBeDefined();
  });

  it("メニューで種類を選ぶと、その種類の選んだときの向きになる", async () => {
    const user = userEvent.setup();
    renderLibrary("/?sort=addedAsc");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "Sort by: Date added" }));
    const items = await screen.findAllByRole("menuitemradio");
    expect(items.map((item) => item.textContent)).toEqual([
      "Date added",
      "Date modified",
      "Date created",
      "Title",
      "Length",
      "File size",
      "Recently played",
      "Date favorited",
      "Random",
    ]);
    await user.click(screen.getByRole("menuitemradio", { name: "File size" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=sizeDesc");
  });

  describe("お気に入りのみと「Date favorited」（specs/035-favorites/ui-design.md「Filter menu」「Sort and direction」）", () => {
    it("「Favorites only」を入れると favorite=true で読み、URL に fav=1 が付き、ボタンの数字に数え、「Clear filters」で外れる（要件 8）", async () => {
      const user = userEvent.setup();
      renderLibrary("/?sort=titleAsc");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter" }));
      const filter = await screen.findByRole("dialog");
      // 視聴状態の直下、「Playable only」の上に置く。
      const checks = within(filter)
        .getAllByRole("checkbox")
        .map((check) => (check as HTMLButtonElement).labels[0]?.textContent);
      expect(checks).toEqual(["Favorites only", "Playable only"]);
      await user.click(within(filter).getByRole("checkbox", { name: "Favorites only" }));

      expect(screen.getByTestId("location").textContent).toBe("?fav=1&sort=titleAsc");
      await waitFor(() =>
        expect(listRequests(fetchMock).at(-1)?.searchParams.get("favorite")).toBe("true"),
      );
      expect(screen.getByRole("button", { name: "Filter (1 applied)" })).toBeDefined();
      expect(
        screen
          .getByRole("button", { name: "Filter (1 applied)" })
          .querySelector('[data-slot="badge"]')?.className,
      ).toContain("bg-primary-soft");

      await user.click(screen.getByRole("button", { name: "Clear filters" }));
      expect(screen.getByTestId("location").textContent).toBe("?sort=titleAsc");
      await waitFor(() =>
        expect(listRequests(fetchMock).at(-1)?.searchParams.has("favorite")).toBe(false),
      );
      expect(screen.getByRole("button", { name: "Filter" })).toBeDefined();
    });

    it("タグの絞り込みと同時に使うと両方を送る（受け入れ条件 9）", async () => {
      renderLibrary("/?fav=1&sort=addedDesc&tag=3");
      await screen.findByRole("link", { name: "動画 1" });
      const last = listRequests(fetchMock).at(-1);
      expect(last?.searchParams.get("favorite")).toBe("true");
      expect(last?.searchParams.getAll("tag")).toEqual(["3"]);
    });

    it("「すべて選択」もお気に入りのみの条件で全件を取る", async () => {
      const base = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname === "/api/library/ids") {
          return Promise.resolve(json({ ids: [1, 2, 3] }));
        }
        return base?.(input, init) ?? Promise.reject(new Error("unexpected"));
      });
      const user = userEvent.setup();
      renderLibrary("/?fav=1&sort=addedDesc");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByText("3 videos selected")).toBeDefined();
      const ids = fetchMock.mock.calls
        .map(([input]) => new URL(String(input), "http://localhost"))
        .find((url) => url.pathname === "/api/library/ids");
      expect(ids?.searchParams.get("favorite")).toBe("true");
    });

    it("「Date favorited」は新しい順で読み、向きの切り替えで古い順になり、URL と端末の設定に残る（要件 10、受け入れ条件 8）", async () => {
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Sort by: Date added" }));
      await user.click(
        await screen.findByRole("menuitemradio", { name: "Date favorited" }),
      );
      expect(screen.getByTestId("location").textContent).toBe("?sort=favoritedDesc");
      await waitFor(() =>
        expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe(
          "favoritedDesc",
        ),
      );
      expect(
        screen.getByRole("button", { name: "Sort by: Date favorited" }),
      ).toBeDefined();
      expect(JSON.parse(localStorage.getItem("vv.view.v2") ?? "{}")).toMatchObject({
        sort: "favoritedDesc",
      });

      await user.click(
        screen.getByRole("button", {
          name: "Descending (newest first). Press for ascending",
        }),
      );
      expect(screen.getByTestId("location").textContent).toBe("?sort=favoritedAsc");
      await waitFor(() =>
        expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe(
          "favoritedAsc",
        ),
      );
      expect(JSON.parse(localStorage.getItem("vv.view.v2") ?? "{}")).toMatchObject({
        sort: "favoritedAsc",
      });
    });

    it("端末に保存した「Date favorited」で開く", async () => {
      localStorage.setItem(
        "vv.view.v2",
        JSON.stringify({ zoom: 1, view: "grid", sort: "favoritedAsc" }),
      );
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe(
        "favoritedAsc",
      );
      expect(
        screen.getByRole("button", { name: "Sort by: Date favorited" }),
      ).toBeDefined();
    });

    it("fav=1 の一覧から動画を開くと fav=1 の鍵で控え、絞らない一覧の鍵では取れない", async () => {
      const { takeListSnapshot } = await import("../api/listSnapshot");
      renderLibrary("/?fav=1&sort=addedDesc");
      await screen.findByRole("link", { name: "動画 1" });
      fireEvent.click(screen.getByRole("link", { name: "動画 1" }));
      await screen.findByText("再生画面");

      expect(
        takeListSnapshot({ query: "", favorite: true, sort: "addedDesc", tags: [] }),
      ).toBeDefined();
      expect(
        takeListSnapshot({ query: "", favorite: false, sort: "addedDesc", tags: [] }),
      ).toBeUndefined();
    });

    it("fav=1 の控えは fav=1 の一覧で使い、ほかの条件が同じ絞らない一覧では使わずに取り直す", async () => {
      const { saveListSnapshot } = await import("../api/listSnapshot");
      saveListSnapshot(
        { query: "", favorite: true, sort: "addedDesc" },
        {
          items: [videoItem(video(9, { title: "控えの動画" }))],
          total: 1,
          hasMore: false,
          scrollY: 0,
        },
      );
      const first = renderLibrary("/?fav=1&sort=addedDesc");
      expect(await screen.findByRole("link", { name: "控えの動画" })).toBeDefined();
      expect(listRequests(fetchMock)).toHaveLength(0);
      first.unmount();

      renderLibrary("/?sort=addedDesc");
      expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
      expect(screen.queryByRole("link", { name: "控えの動画" })).toBeNull();
      expect(listRequests(fetchMock).length).toBeGreaterThan(0);
    });
  });

  it("「Date created」は新しい順で読み、向きの切り替えで古い順になり、URL と端末の設定に残る", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "Sort by: Date added" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Date created" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=createdDesc");
    await waitFor(() =>
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe(
        "createdDesc",
      ),
    );
    expect(screen.getByRole("button", { name: "Sort by: Date created" })).toBeDefined();
    expect(JSON.parse(localStorage.getItem("vv.view.v2") ?? "{}")).toMatchObject({
      sort: "createdDesc",
    });

    await user.click(
      screen.getByRole("button", {
        name: "Descending (newest first). Press for ascending",
      }),
    );
    expect(screen.getByTestId("location").textContent).toBe("?sort=createdAsc");
    await waitFor(() =>
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe("createdAsc"),
    );
    expect(JSON.parse(localStorage.getItem("vv.view.v2") ?? "{}")).toMatchObject({
      sort: "createdAsc",
    });
  });

  it("URL の sort=createdAsc を読み、作成日の古い順で要求する", async () => {
    renderLibrary("/?sort=createdAsc");
    await screen.findByRole("link", { name: "動画 1" });
    await waitFor(() =>
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe("createdAsc"),
    );
    expect(screen.getByRole("button", { name: "Sort by: Date created" })).toBeDefined();
    expect(
      screen.getByRole("button", {
        name: "Ascending (oldest first). Press for descending",
      }),
    ).toBeDefined();
  });

  it("検索語の入力は一続きで履歴を1つだけ増やす", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });

    const box = screen.getByRole("searchbox", { name: "Search videos" });
    await user.type(box, "京");
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe(
        `?q=${encodeURIComponent("京")}&sort=addedDesc`,
      ),
    );
    await user.type(box, "都");
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe(
        `?q=${encodeURIComponent("京都")}&sort=addedDesc`,
      ),
    );

    // Esc で抜けると続きが閉じる。戻ると入力を始める前の一覧になる。
    await user.click(screen.getByRole("button", { name: "テストで戻る" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=addedDesc");
    expect((box as HTMLInputElement).value).toBe("");
  });

  it("一致なしでは条件や解除操作を重ねない", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      const params = new URL(url, "http://localhost").searchParams;
      const conditioned =
        params.has("query") || params.has("watch") || params.has("playable");
      return Promise.resolve(
        json(
          conditioned
            ? ({ items: [], total: 0 } satisfies VideoPage)
            : ({ items: [video(1)], total: 1 } satisfies VideoPage),
        ),
      );
    });
    renderLibrary("/?q=%E4%BA%AC%E9%83%BD&watch=unwatched&playable=1&sort=titleDesc");

    expect(await screen.findByText("No videos match these conditions")).toBeDefined();
    expect(screen.queryByText("検索語「京都」")).toBeNull();
    expect(screen.queryByText("Unwatched")).toBeNull();
    expect(screen.queryByText("Playable only")).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
    expect(screen.queryByText("No videos yet")).toBeNull();
    await userEvent.setup().click(screen.getByRole("button", { name: "Change search" }));
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "Search videos" }),
    );
  });

  it("絞り込みの条件を解除は検索語も外し、ポップオーバーを閉じて絞り込みのボタンへ戻る", async () => {
    const user = userEvent.setup();
    renderLibrary("/?q=abc&sort=titleAsc");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(await screen.findByRole("button", { name: "Clear filters" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTestId("location").textContent).toBe("?sort=titleAsc");
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Filter" })),
    );
  });

  it("キーボードだけで検索欄 → × → 手引き → 絞り込み → 並べ替え → 向きの順に進み、手引きは Tab で閉じる", async () => {
    const user = userEvent.setup();
    renderLibrary("/?q=abc&sort=addedDesc");
    await screen.findByRole("link", { name: "動画 1" });

    const box = screen.getByRole("searchbox", { name: "Search videos" });
    const help = screen.getByRole("button", { name: "How to search" });
    box.focus();
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Clear search" }),
    );
    await user.tab();
    expect(document.activeElement).toBe(help);

    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getAllByRole("term")).toHaveLength(4);
    expect(document.activeElement).toBe(help);

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Filter" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect((box as HTMLInputElement).value).toBe("abc");
    expect(screen.getByTestId("location").textContent).toBe("?q=abc&sort=addedDesc");

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Sort by: Date added" }),
    );
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", {
        name: "Descending (newest first). Press for ascending",
      }),
    );
  });

  it("手引きを開いて閉じても検索語と URL は変わらない", async () => {
    const user = userEvent.setup();
    renderLibrary("/?q=abc&sort=addedDesc");
    await screen.findByRole("link", { name: "動画 1" });

    const help = screen.getByRole("button", { name: "How to search" });
    await user.click(help);
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    expect(document.activeElement).toBe(help);
    expect(
      (screen.getByRole("searchbox", { name: "Search videos" }) as HTMLInputElement)
        .value,
    ).toBe("abc");
    expect(screen.getByTestId("location").textContent).toBe("?q=abc&sort=addedDesc");
  });

  it("未視聴で絞った一覧から戻ったとき、再生位置が変わった項目もその場に残す", async () => {
    const { recordSavedProgress, nextProgressSequence } =
      await import("../api/progressEvents");
    renderLibrary("/?watch=unwatched");
    await screen.findByRole("link", { name: "動画 1" });
    act(() =>
      recordSavedProgress(
        1,
        { positionMs: 30_000, completed: false, updatedAt: "" },
        nextProgressSequence(),
      ),
    );
    expect(screen.getByRole("link", { name: "動画 1" })).toBeDefined();
  });

  it("一部にしか反映されなかった切り替えの取り直し中に動画を開いても、古い一覧を控えない（PR 338）", async () => {
    const { saveListSnapshot, takeListSnapshot } = await import("../api/listSnapshot");
    const { updateVideoVisibility } = await import("../api/visibility");
    const base = fetchMock.getMockImplementation();
    let finishRefetch: (() => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/video-visibility") return Promise.resolve(json({ applied: 1 }));
      if (url === "/api/videos/1") {
        // 取り直しを止めておき、その間に動画を開く。
        return new Promise<Response>((resolve) => {
          finishRefetch = () => resolve(json(video(1, { public: true })));
        });
      }
      if (url === "/api/videos/2") return Promise.resolve(json(video(2)));
      return base?.(input, init) ?? Promise.reject(new Error("unexpected"));
    });
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });

    await act(async () => {
      await updateVideoVisibility([1, 2], true);
    });
    await waitFor(() => expect(finishRefetch).toBeDefined());
    fireEvent.click(screen.getByRole("link", { name: "動画 1" }));
    await screen.findByText("再生画面");

    // 取り直す前の public: false の一覧を控えると、戻ったときに復元されてしまう。
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    // 一覧を離れれば取り直しは打ち切られ、控えを止める印も解ける。
    finishRefetch?.();
    saveListSnapshot(
      { query: "" },
      { items: [videoItem(video(1))], total: 1, hasMore: false, scrollY: 0 },
    );
    expect(takeListSnapshot({ query: "" })).toBeDefined();
  });

  it("選択すると選択バーが出て Esc で消える", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    const checkbox = screen.getByRole("checkbox", { name: 'Select "動画 1"' });
    expect(checkbox.parentElement?.className).toContain(
      "[@media(hover:none)]:opacity-100",
    );
    await user.click(checkbox);
    expect(screen.getByText("1 video selected")).toBeDefined();
    await user.keyboard("{Escape}");
    expect(screen.queryByText("1 video selected")).toBeNull();
  });

  it("preview は一度に1件だけ active にし resize で全 card を reset する", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      return Promise.resolve(
        json({
          items: [
            video(1, { previewState: "done", previewUrl: "/preview-1.mp4" }),
            video(2, { previewState: "done", previewUrl: "/preview-2.mp4" }),
          ],
          total: 2,
        } satisfies VideoPage),
      );
    });
    const play = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    vi.useFakeTimers();

    const cards = screen.getAllByRole("article");
    fireEvent.pointerEnter(cards[0]!, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelectorAll("video")).toHaveLength(1);
    expect(document.querySelector("video")?.getAttribute("src")).toBe("/preview-1.mp4");

    fireEvent.pointerEnter(cards[1]!, { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelectorAll("video")).toHaveLength(1);
    expect(document.querySelector("video")?.getAttribute("src")).toBe("/preview-2.mp4");

    fireEvent(window, new Event("resize"));
    expect(document.querySelector("video")).toBeNull();
    expect(play).toHaveBeenCalledTimes(2);
  });

  it("filter、sort、view、zoom の変更で active preview を reset する", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      return Promise.resolve(
        json({
          items: [
            video(1, { previewState: "done", previewUrl: "/preview-1.mp4" }),
            video(2, { previewState: "done", previewUrl: "/preview-2.mp4" }),
          ],
          total: 2,
        } satisfies VideoPage),
      );
    });
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    vi.useFakeTimers();

    const startFirst = () => {
      fireEvent.pointerEnter(screen.getAllByRole("article")[0]!, {
        pointerType: "mouse",
      });
      act(() => vi.advanceTimersByTime(400));
      expect(document.querySelector("video")).not.toBeNull();
    };

    startFirst();
    fireEvent.click(screen.getByRole("button", { name: "Filter" }));
    fireEvent.click(screen.getByRole("radio", { name: "Unwatched" }));
    await act(async () => Promise.resolve());
    expect(document.querySelector("video")).toBeNull();

    startFirst();
    fireEvent.click(screen.getByRole("button", { name: "View and sort" }));
    let dialog = screen.getByRole("dialog");
    const slider = within(dialog).getByRole("slider", { name: "Card size" });
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(document.querySelector("video")).toBeNull();

    startFirst();
    fireEvent.click(within(dialog).getByRole("radio", { name: "List" }));
    expect(document.querySelector("video")).toBeNull();
    expect(screen.queryByRole("article")).toBeNull();

    dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("radio", { name: "Grid" }));
    startFirst();
    dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("radio", { name: "Title" }));
    expect(document.querySelector("video")).toBeNull();
  });

  it("次 page の読み込み開始と list 追加で active preview を reset する", async () => {
    let intersect: IntersectionObserverCallback | undefined;
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        constructor(callback: IntersectionObserverCallback) {
          intersect = callback;
        }
        observe() {}
        disconnect() {}
      },
    );
    let listCalls = 0;
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      listCalls += 1;
      return Promise.resolve(
        json(
          listCalls === 1
            ? {
                items: [
                  video(1, {
                    previewState: "done",
                    previewUrl: "/preview-1.mp4",
                  }),
                ],
                total: 2,
                nextCursor: "next",
              }
            : { items: [video(2)], total: 2 },
        ),
      );
    });
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    vi.useFakeTimers();
    fireEvent.pointerEnter(screen.getByRole("article"), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(400));
    expect(document.querySelector("video")).not.toBeNull();

    act(() =>
      intersect?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      ),
    );
    expect(document.querySelector("video")).toBeNull();
    vi.useRealTimers();
    expect(await screen.findByRole("link", { name: "動画 2" })).toBeDefined();
    expect(listCalls).toBe(2);
  });

  describe("タグ絞り込み（issue 269）", () => {
    function tagsResponse(tags: { id: number; name: string }[]) {
      return json({
        items: tags.map((tag) => ({ ...tag, synonyms: [], videoCount: 1 })),
      });
    }

    function installTagAwareList(
      tags: { id: number; name: string }[],
      makeItems: (requestedTag: number[]) => {
        items: Video[];
        total: number;
        missingTagIds?: number[];
      },
    ) {
      fetchMock.mockImplementation((input) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
        if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
        if (url.pathname === "/api/processing") {
          return Promise.resolve(
            json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
          );
        }
        if (url.pathname === "/api/tags") return Promise.resolve(tagsResponse(tags));
        if (url.pathname === "/api/library") {
          const requestedTag = url.searchParams.getAll("tag").map(Number);
          return Promise.resolve(json(makeItems(requestedTag) satisfies VideoPage));
        }
        throw new Error(`unexpected request: ${url.toString()}`);
      });
    }

    it("/?tag=1&sort=random は seed を補い、tag=1 を残す", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      renderLibrary("/?tag=1&sort=random");
      await screen.findByRole("link", { name: "動画 1" });

      await waitFor(() => {
        const location = screen.getByTestId("location").textContent ?? "";
        expect(location).toMatch(/tag=1/);
        expect(location).toMatch(/sort=random/);
        expect(location).toMatch(/seed=\d+/);
      });
    });

    it("タグの絞り込みを変えると選択を解除する", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?tag=1");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      expect(screen.getByText("1 video selected")).toBeDefined();

      // 一覧の上のチップでタグの絞り込みを外すと、同じ動画がまだ見えていても
      // 選択は解除する（検索語・視聴状態・再生可否と同じ扱い）。
      await user.click(
        screen.getByRole("button", { name: "Remove the filter for 旅行" }),
      );
      await waitFor(() => expect(screen.queryByText("1 video selected")).toBeNull());
    });

    it("カードのタグを押すと絞り込みに加わり、上の行に出る。すでに絞り込み中のタグは変わらない", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tag], (requested) => ({
        items:
          requested.length === 0
            ? [video(1, { tags: [tag] })]
            : [video(1, { tags: [tag] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter by 旅行" }));
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).toContain("tag=1"),
      );
      expect(
        within(screen.getByRole("list", { name: "Tags in the filter" })).getByText(
          "旅行",
        ),
      ).toBeDefined();

      // すでに絞り込み中のタグをカードでもう一度押しても何も変わらない。
      const before = screen.getByTestId("location").textContent;
      await user.click(screen.getByRole("button", { name: "Filter by 旅行" }));
      expect(screen.getByTestId("location").textContent).toBe(before);
    });

    it("タグを押しても、検索語・視聴状態などのほかの条件は残る（N1）", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?q=abc&watch=unwatched");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter by 旅行" }));

      await waitFor(() => {
        const location = screen.getByTestId("location").textContent ?? "";
        expect(location).toContain("tag=1");
        expect(location).toContain("q=abc");
        expect(location).toContain("watch=unwatched");
      });
      // 一覧の要求にも、タグと一緒にほかの条件が残る。
      await waitFor(() => {
        const request = listRequests(fetchMock).at(-1);
        expect(request?.searchParams.get("query")).toBe("abc");
        expect(request?.searchParams.get("watch")).toBe("unwatched");
        expect(request?.searchParams.getAll("tag")).toEqual(["1"]);
      });
    });

    it("16個絞り込んでいるときに17個目を押すと、加えずにトーストで伝える", async () => {
      const extra = {
        id: 17,
        name: "17個目",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([extra], () => ({
        items: [video(1, { tags: [extra] })],
        total: 1,
      }));
      const user = userEvent.setup();
      const search = Array.from(
        { length: 16 },
        (_, index) => `tag=${String(index + 1)}`,
      ).join("&");
      renderLibrary(`/?${search}`);
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter by 17個目" }));
      expect(await screen.findByText("Filter by at most 16 tags.")).toBeDefined();
      expect(screen.getByTestId("location").textContent).not.toContain("tag=17");
    });

    it("絞り込み中のタグを外すと、その id だけが消えてほかの条件は残る", async () => {
      const tagA = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      const tagB = {
        id: 2,
        name: "2024",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tagA, tagB], () => ({
        items: [video(1, { tags: [tagA, tagB] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?tag=1&tag=2&q=abc");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(
        screen.getByRole("button", { name: "Remove the filter for 2024" }),
      );
      await waitFor(() => {
        const location = screen.getByTestId("location").textContent ?? "";
        expect(location).toContain("tag=1");
        expect(location).not.toContain("tag=2&");
        expect(location).not.toMatch(/tag=2$/);
        expect(location).toContain("q=abc");
      });
    });

    it("missingTagIds を受けたら伝えて、タグの一覧を取り直し、一覧も取り直して URL から取り除く（N1）", async () => {
      const tagA = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tagA], (requested) =>
        requested.includes(1)
          ? { items: [], total: 0, missingTagIds: [1] }
          : { items: [video(1)], total: 1 },
      );
      renderLibrary("/?tag=1");
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).not.toContain("tag=1"),
      );
      expect(
        await screen.findByText("Removed deleted tags from the filter"),
      ).toBeDefined();

      // タグの一覧（/api/tags）を取り直す。
      await waitFor(() => {
        const tagRequests = fetchMock.mock.calls.filter((call) =>
          new URL(String(call[0]), "http://localhost").pathname.startsWith("/api/tags"),
        );
        expect(tagRequests.length).toBeGreaterThanOrEqual(2);
      });
      // 一覧（/api/library）も、tag を外した条件で取り直す。
      await waitFor(() => {
        const videoRequests = fetchMock.mock.calls.filter((call) => {
          const url = new URL(String(call[0]), "http://localhost");
          return url.pathname === "/api/library" && !url.searchParams.has("tag");
        });
        expect(videoRequests.length).toBeGreaterThanOrEqual(1);
      });
    });

    it("控えから戻したときも、共有のタグの一覧と突き合わせてもう無い id を取り除く。一覧も取り直す（N1）", async () => {
      const { saveListSnapshot } = await import("../api/listSnapshot");
      saveListSnapshot(
        { query: "", sort: "addedDesc", tags: [1] },
        { items: [videoItem(video(1))], total: 1, hasMore: false, scrollY: 0 },
      );
      // 共有のタグの一覧にはもう id 1 が無い（別のタブで削除された想定）。
      installTagAwareList([], () => ({ items: [video(1)], total: 1 }));
      // sort を明示し、端末に保存された並び順（前のテストの操作の影響）に
      // 依らず控えの鍵を "addedDesc" に固定する。
      renderLibrary("/?tag=1&sort=addedDesc");

      // 控えを使うので、最初の /api/library は要求しない。
      expect(
        fetchMock.mock.calls.filter(
          (call) =>
            new URL(String(call[0]), "http://localhost").pathname === "/api/library",
        ),
      ).toHaveLength(0);

      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).not.toContain("tag=1"),
      );
      expect(
        await screen.findByText("Removed deleted tags from the filter"),
      ).toBeDefined();

      // タグの一覧を取り直し（画面が開くとき + 突き合わせで最低2回）、条件が
      // 変わったので一覧（/api/library）も取り直す。
      await waitFor(() => {
        const tagRequests = fetchMock.mock.calls.filter((call) =>
          new URL(String(call[0]), "http://localhost").pathname.startsWith("/api/tags"),
        );
        expect(tagRequests.length).toBeGreaterThanOrEqual(1);
      });
      await waitFor(() => {
        const videoRequests = fetchMock.mock.calls.filter(
          (call) =>
            new URL(String(call[0]), "http://localhost").pathname === "/api/library",
        );
        expect(videoRequests.length).toBeGreaterThanOrEqual(1);
      });
    });

    it("/ の控えは /?tag=1 のマウントでは使われない（listSnapshot の鍵に tags が要る）", async () => {
      const { saveListSnapshot } = await import("../api/listSnapshot");
      // tag の無い「/」の控えを残しておく。
      saveListSnapshot(
        { query: "", sort: "addedDesc" },
        {
          items: [videoItem(video(99, { title: "控えの動画" }))],
          total: 1,
          hasMore: false,
          scrollY: 0,
        },
      );
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));

      renderLibrary("/?tag=1");

      // 控え（控えの動画）ではなく、要求した一覧（動画 1）が出る。
      expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
      expect(screen.queryByRole("link", { name: "控えの動画" })).toBeNull();
      // 控えを使わないので、通常どおり /api/library を要求する。
      expect(
        fetchMock.mock.calls.some(
          (call) =>
            new URL(String(call[0]), "http://localhost").pathname === "/api/library",
        ),
      ).toBe(true);
    });

    it("タグだけで絞って0件のとき、該当なしにタグのチップが出て、条件を解除でタグが外れる", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installTagAwareList([tag], () => ({ items: [], total: 0 }));
      const user = userEvent.setup();
      renderLibrary("/?tag=1");

      expect(await screen.findByText("No videos match these conditions")).toBeDefined();
      expect(
        within(screen.getByRole("list", { name: "Tags in the filter" })).getByText(
          "旅行",
        ),
      ).toBeDefined();

      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(await screen.findByRole("button", { name: "Clear filters" }));
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).not.toContain("tag="),
      );
    });
  });

  describe("選択バーのタグ一括操作・すべて選択（issue 270）", () => {
    function installSelectionAwareList(options: {
      tags?: { id: number; name: string }[];
      total?: number;
      allIds?: number[];
      idsMissingTagIds?: number[];
      idsFail?: boolean;
      /** true にすると /api/library/ids の応答を、呼び出し元が明示的に流すまで止める。 */
      idsDelay?: boolean;
    }) {
      const tags = options.tags ?? [];
      const total = options.total ?? 3;
      const attached = new Map<number, Set<number>>();
      let resolveIds: (() => void) | undefined;
      const idsGate = new Promise<void>((resolve) => {
        resolveIds = resolve;
      });
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
        if (url.pathname === "/api/tags" && method === "GET") {
          return Promise.resolve(
            json({ items: tags.map((tag) => ({ ...tag, synonyms: [], videoCount: 1 })) }),
          );
        }
        if (url.pathname === "/api/library/ids") {
          const respond = () => {
            if (options.idsFail) return Promise.resolve(json({ code: "internal" }, 500));
            if (options.idsMissingTagIds !== undefined) {
              return Promise.resolve(
                json({ ids: [], missingTagIds: options.idsMissingTagIds }),
              );
            }
            return Promise.resolve(json({ ids: options.allIds ?? [] }));
          };
          return options.idsDelay === true ? idsGate.then(respond) : respond();
        }
        if (url.pathname === "/api/video-tags" && method === "POST") {
          const body = JSON.parse(String(init?.body)) as {
            videoIds: number[];
            action: "add" | "remove";
            tag: { id: number } | { name: string };
          };
          const requestedTag = body.tag;
          const found =
            "id" in requestedTag ? tags.find((t) => t.id === requestedTag.id) : undefined;
          const resolvedTag = found ?? {
            id: 999,
            name: "id" in requestedTag ? "?" : requestedTag.name,
          };
          for (const videoId of body.videoIds) {
            const set = attached.get(videoId) ?? new Set<number>();
            if (body.action === "add") set.add(resolvedTag.id);
            else set.delete(resolvedTag.id);
            attached.set(videoId, set);
          }
          return Promise.resolve(
            json({ tag: resolvedTag, applied: body.videoIds.length }),
          );
        }
        if (url.pathname === "/api/video-tags/summary" && method === "POST") {
          const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
          const counts = new Map<number, number>();
          for (const videoId of body.videoIds) {
            for (const tagId of attached.get(videoId) ?? []) {
              counts.set(tagId, (counts.get(tagId) ?? 0) + 1);
            }
          }
          const items = [...counts.entries()].map(([tagId, count]) => ({
            tag: { id: tagId, name: tags.find((t) => t.id === tagId)?.name ?? "?" },
            count,
            manualCount: count,
          }));
          return Promise.resolve(json({ total: body.videoIds.length, items }));
        }
        if (url.pathname === "/api/library") {
          return Promise.resolve(
            json({
              items: [
                video(1, {
                  tags: [...(attached.get(1) ?? [])].map((id) => ({
                    id,
                    name: "旅行",
                    manual: true,
                    fromFolder: false,
                    tentative: false,
                  })),
                }),
                video(2),
                video(3),
              ],
              total,
            } satisfies VideoPage),
          );
        }
        throw new Error(`unexpected request: ${url.toString()}`);
      });
      return { resolveIds: () => resolveIds?.() };
    }

    it("すべて選択で、読み込んでいないページを含む全件が選ばれる（受け入れ条件4）", async () => {
      installSelectionAwareList({
        total: 50,
        allIds: Array.from({ length: 50 }, (_, i) => i + 1),
      });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      expect(screen.getByText("1 video selected")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByText("50 videos selected")).toBeDefined();
    });

    it("すべて選択が失敗したら、選択を変えずにトーストで伝える", async () => {
      installSelectionAwareList({ total: 50, idsFail: true });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));

      expect(
        await screen.findByText(
          "Couldn't select everything: Something went wrong on the server.",
        ),
      ).toBeDefined();
      expect(screen.getByText("1 video selected")).toBeDefined();
    });

    // Devin の指摘1: すべて選択の要求中に手動で選択を変えると、その要求は無効に
    // なる（selectAllSeq が進む）。以前は、その無効になった要求の応答（や
    // finally）が selectAllSeq の不一致で素通りし、selectingAll を false に
    // 戻す機会が無いまま「選択中…」に固まっていた。
    it("すべて選択の要求中に選択を手動で変えると、選択中…のまま固まらない", async () => {
      const { resolveIds } = installSelectionAwareList({
        total: 50,
        allIds: Array.from({ length: 50 }, (_, i) => i + 1),
        idsDelay: true,
      });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByRole("button", { name: "Selecting…" })).toBeDefined();

      // 応答がまだ届かない間に、手動で選択を変える（この要求はもう当てはまらない）。
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));

      // ボタンは「選択中…」で固まらず、「すべて選択」に戻る。
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "Select all" })).toBeDefined(),
      );
      expect(screen.getByText("2 videos selected")).toBeDefined();

      // 遅れて届いた応答（もう無効）は、手動で選んだ2件を上書きしない。
      await act(async () => {
        resolveIds();
        await Promise.resolve();
      });
      expect(screen.getByText("2 videos selected")).toBeDefined();
    });

    it("絞り込み中のタグを別のタブで消してから「すべて選択」すると、選ばれず、もう無いことが伝わる", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installSelectionAwareList({ tags: [], total: 3, idsMissingTagIds: [1] });
      const user = userEvent.setup();
      renderLibrary("/?tag=1");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));

      expect(
        await screen.findByText("Removed deleted tags from the filter"),
      ).toBeDefined();
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).not.toContain("tag=1"),
      );
      // 選択は作られない。条件（tag）が変わったことで、押す前の選択も解除される
      // （検索語・視聴状態・再生可否と同じ扱い。ui-design.md「Active tag filters」）。
      await waitFor(() =>
        expect(screen.queryByText(/^\d[\d,]* videos? selected$/)).toBeNull(),
      );
      void tag;
    });

    it("選択バーでタグを付けると、読み込み済みのカードにすぐ出て、選択は残る", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      installSelectionAwareList({ tags: [tag] });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
      await user.click(screen.getByRole("button", { name: "Add tag" }));
      const input = await screen.findByRole("combobox", { name: "Add tag" });
      await user.type(input, "旅行");
      await screen.findByRole("option", { name: /旅行/ });
      await user.keyboard("{Enter}");

      expect(await screen.findByText('Added "旅行" to 2 videos')).toBeDefined();
      // 選択は残る。
      expect(screen.getByText("2 videos selected")).toBeDefined();
      // 読み込み済みのカード（動画1）にタグがすぐ出る（#267 の通知）。可視の行
      // （タグの一覧）だけを見る。オーバーフロー計測用の隠れた複製とは別に数える。
      const card = screen.getByText("動画 1").closest("article");
      expect(card).not.toBeNull();
      const tagList = within(card as HTMLElement).getByRole("list", { name: "Tags" });
      expect(within(tagList).getByText("旅行")).toBeDefined();
    });

    it("Esc で選択バーのポップオーバーだけを閉じ、選択は残る（キーボード確認 手順2）", async () => {
      installSelectionAwareList({ tags: [{ id: 1, name: "旅行" }] });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Remove tag" }));
      await screen.findByText("The selected videos have no tags that can be removed");

      await user.keyboard("{Escape}");
      await waitFor(() =>
        expect(
          screen.queryByText("The selected videos have no tags that can be removed"),
        ).toBeNull(),
      );
      // ポップオーバーだけが閉じ、選択バー自体（選択）は残る。
      expect(screen.getByText("1 video selected")).toBeDefined();
    });

    // B1: 以前は items が変わるたびに、選択を items に無い id ごと刈り込んで
    // いた。タグの付け外しも loadMore も items を新しい配列に置き換えるので、
    // 「すべて選択」でまだ読み込んでいない id まで選んでいると、それらが
    // 巻き込まれて消えていた。
    it("すべて選択のあとタグを付けても、選択の件数は変わらない（B1）", async () => {
      installSelectionAwareList({
        tags: [{ id: 1, name: "旅行" }],
        total: 50,
        allIds: Array.from({ length: 50 }, (_, i) => i + 1),
      });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByText("50 videos selected")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "Add tag" }));
      const input = await screen.findByRole("combobox", { name: "Add tag" });
      await user.type(input, "旅行");
      await screen.findByRole("option", { name: /旅行/ });
      await user.keyboard("{Enter}");

      expect(await screen.findByText('Added "旅行" to 50 videos')).toBeDefined();
      // 読み込み済みのカードにタグが反映されて items が新しい配列になっても、
      // 選択の件数（読み込んでいない分を含む）はそのまま。
      expect(screen.getByText("50 videos selected")).toBeDefined();
    });

    it("すべて選択のあと loadMore しても、選択の件数は変わらない（B1）", async () => {
      let intersect: IntersectionObserverCallback | undefined;
      vi.stubGlobal(
        "IntersectionObserver",
        class {
          constructor(callback: IntersectionObserverCallback) {
            intersect = callback;
          }
          observe() {}
          disconnect() {}
        },
      );
      let listCalls = 0;
      fetchMock.mockImplementation((input) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
        if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
        if (url.pathname === "/api/processing") {
          return Promise.resolve(
            json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
          );
        }
        if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
        if (url.pathname === "/api/library/ids") {
          return Promise.resolve(json({ ids: [1, 2, 3, 4, 5] }));
        }
        if (url.pathname === "/api/library") {
          listCalls += 1;
          return Promise.resolve(
            json(
              listCalls === 1
                ? {
                    items: [video(1), video(2), video(3)],
                    total: 5,
                    nextCursor: "next",
                  }
                : { items: [video(4), video(5)], total: 5 },
            ),
          );
        }
        throw new Error(`unexpected request: ${url.toString()}`);
      });

      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByText("5 videos selected")).toBeDefined();

      act(() =>
        intersect?.(
          [{ isIntersecting: true } as IntersectionObserverEntry],
          {} as IntersectionObserver,
        ),
      );
      expect(await screen.findByRole("link", { name: "動画 4" })).toBeDefined();
      // loadMore で items が伸びても、選択の件数はそのまま。
      expect(screen.getByText("5 videos selected")).toBeDefined();
    });

    // Devin の指摘4: 一括で外したタグが今の絞り込みに含まれているときは、選択を
    // 解除して一覧を取り直し、件数と一覧を条件に合わせ直す
    // （docs/design-docs/library-ui.md §6）。
    it("一括で外したタグが今の絞り込みに含まれるとき、選択を解除して一覧を取り直す（Devin の指摘4）", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
      };
      const attached = new Map<number, Set<number>>([[1, new Set([1])]]);
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
        if (url.pathname === "/api/tags" && method === "GET") {
          return Promise.resolve(
            json({
              items: [{ ...tag, synonyms: [], videoCount: attached.get(1)?.size ?? 0 }],
            }),
          );
        }
        if (url.pathname === "/api/video-tags" && method === "POST") {
          const body = JSON.parse(String(init?.body)) as {
            videoIds: number[];
            action: "add" | "remove";
          };
          for (const videoId of body.videoIds) {
            const set = attached.get(videoId) ?? new Set<number>();
            if (body.action === "add") set.add(1);
            else set.delete(1);
            attached.set(videoId, set);
          }
          return Promise.resolve(json({ tag, applied: body.videoIds.length }));
        }
        if (url.pathname === "/api/video-tags/summary" && method === "POST") {
          const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
          const count = body.videoIds.filter((id) => attached.get(id)?.has(1)).length;
          return Promise.resolve(
            json({
              total: body.videoIds.length,
              items: count > 0 ? [{ tag, count, manualCount: count }] : [],
            }),
          );
        }
        if (url.pathname === "/api/library") {
          const requestedTags = url.searchParams.getAll("tag").map(Number);
          const matching = [1, 2, 3].filter((id) =>
            requestedTags.every((t) => attached.get(id)?.has(t)),
          );
          return Promise.resolve(
            json({
              items: matching.map((id) =>
                video(id, { tags: attached.get(id)?.has(1) ? [tag] : [] }),
              ),
              total: matching.length,
            } satisfies VideoPage),
          );
        }
        throw new Error(`unexpected request: ${url.toString()}`);
      });

      const user = userEvent.setup();
      renderLibrary("/?tag=1");
      await screen.findByRole("link", { name: "動画 1" });
      expect(screen.getByRole("article")).toBeDefined();

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      expect(screen.getByText("1 video selected")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "Remove tag" }));
      const removeOption = await screen.findByRole("option", { name: /旅行/ });
      await user.click(removeOption);
      expect(await screen.findByText('Removed "旅行" from 1 video')).toBeDefined();

      // 選択は解除され、絞り込み（tag=1）に合う動画が無くなった一覧に取り直す。
      await waitFor(() =>
        expect(screen.queryByText(/^\d[\d,]* videos? selected$/)).toBeNull(),
      );
      expect(await screen.findByText("No videos match these conditions")).toBeDefined();
    });
  });
  describe("ゲスト（specs/016-single-account-auth/ui-design.md「Guest degradation」）", () => {
    function guestPage(): VideoPage {
      // ゲストの応答には progress が無く、tags は空である。
      return {
        items: [video(1, { public: true }), video(2, { public: true })],
        total: 2,
      };
    }

    beforeEach(() => {
      installFakeEventSource();
      fetchMock.mockImplementation(() => Promise.resolve(json(guestPage())));
    });

    it("選択・選択バー・視聴状態・最近再生した順を出さず、所有者だけの API を呼ばない", async () => {
      const user = userEvent.setup();
      renderLibrary("/", "guest");
      expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
      expect(screen.queryByRole("checkbox")).toBeNull();

      await user.click(screen.getByRole("button", { name: "Filter" }));
      const filter = await screen.findByRole("dialog");
      expect(within(filter).queryByText("Watch status")).toBeNull();
      expect(within(filter).queryByRole("radio")).toBeNull();
      expect(
        within(filter).getByRole("checkbox", { name: "Playable only" }),
      ).toBeDefined();
      await user.keyboard("{Escape}");

      await user.click(screen.getByRole("button", { name: /^Sort by:/ }));
      const menu = await screen.findByRole("menu");
      expect(within(menu).getAllByRole("menuitemradio")).toHaveLength(7);
      expect(within(menu).queryByText("Recently played")).toBeNull();
      expect(
        within(menu).getByRole("menuitemradio", { name: "Date created" }),
      ).toBeDefined();

      const paths = fetchMock.mock.calls.map(
        ([input]) => new URL(String(input), "http://localhost").pathname,
      );
      expect(paths.every((path) => path === "/api/library")).toBe(true);
      expect(FakeEventSource.instances).toHaveLength(0);
    });

    it("URL に残った watch・最近再生した順・tag は既定に丸めて要求し、URL も直す", async () => {
      renderLibrary("/?q=ab&watch=unwatched&sort=playedDesc&tag=3", "guest");
      await screen.findByRole("link", { name: "動画 1" });
      const requests = listRequests(fetchMock);
      expect(requests.length).toBeGreaterThan(0);
      for (const url of requests) {
        expect(url.searchParams.get("watch") ?? "all").toBe("all");
        expect(url.searchParams.get("sort")).toBe("addedDesc");
        expect(url.searchParams.getAll("tag")).toEqual([]);
        expect(url.searchParams.get("query")).toBe("ab");
      }
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).toBe("?q=ab&sort=addedDesc"),
      );
    });

    it("「Favorites only」と「Date favorited」を出さない（受け入れ条件 10）", async () => {
      const user = userEvent.setup();
      renderLibrary("/", "guest");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter" }));
      const filter = await screen.findByRole("dialog");
      expect(
        within(filter).queryByRole("checkbox", { name: "Favorites only" }),
      ).toBeNull();
      await user.keyboard("{Escape}");

      await user.click(screen.getByRole("button", { name: /^Sort by:/ }));
      const menu = await screen.findByRole("menu");
      expect(within(menu).queryByText("Date favorited")).toBeNull();
      expect(
        within(menu)
          .getAllByRole("menuitemradio")
          .map((item) => item.textContent),
      ).toEqual([
        "Date added",
        "Date modified",
        "Date created",
        "Title",
        "Length",
        "File size",
        "Random",
      ]);
    });

    it("URL に残った fav=1 と favorited* は既定に丸めて要求し、URL も直す（受け入れ条件 10）", async () => {
      for (const sort of ["favoritedDesc", "favoritedAsc"]) {
        fetchMock.mockClear();
        const view = renderLibrary(`/?q=ab&fav=1&sort=${sort}`, "guest");
        await screen.findByRole("link", { name: "動画 1" });
        const requests = listRequests(fetchMock);
        expect(requests.length).toBeGreaterThan(0);
        for (const url of requests) {
          expect(url.searchParams.has("favorite")).toBe(false);
          expect(url.searchParams.get("sort")).toBe("addedDesc");
          expect(url.searchParams.get("query")).toBe("ab");
        }
        await waitFor(() =>
          expect(screen.getByTestId("location").textContent).toBe("?q=ab&sort=addedDesc"),
        );
        view.unmount();
      }
    });

    it("端末に保存した並び順が「Date favorited」でも既定で要求し、保存値は書き換えない", async () => {
      localStorage.setItem(
        "vv.view.v2",
        JSON.stringify({ zoom: 1, view: "grid", sort: "favoritedDesc" }),
      );
      renderLibrary("/", "guest");
      await screen.findByRole("link", { name: "動画 1" });
      for (const url of listRequests(fetchMock)) {
        expect(url.searchParams.get("sort")).toBe("addedDesc");
      }
      expect(JSON.parse(localStorage.getItem("vv.view.v2") ?? "{}")).toMatchObject({
        sort: "favoritedDesc",
      });
    });

    it("端末に保存した並び順が最近再生した順でも既定で要求し、保存値は書き換えない", async () => {
      localStorage.setItem(
        "vv.view.v2",
        JSON.stringify({ zoom: 1, view: "grid", sort: "playedDesc" }),
      );
      renderLibrary("/", "guest");
      await screen.findByRole("link", { name: "動画 1" });
      for (const url of listRequests(fetchMock)) {
        expect(url.searchParams.get("sort")).toBe("addedDesc");
      }
      expect(JSON.parse(localStorage.getItem("vv.view.v2") ?? "{}")).toMatchObject({
        sort: "playedDesc",
      });
    });

    it("リスト表示の行に選択のチェックを出さない", async () => {
      localStorage.setItem(
        "vv.view.v2",
        JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
      );
      renderLibrary("/", "guest");
      await screen.findByRole("link", { name: "動画 1" });
      expect(screen.queryByRole("checkbox")).toBeNull();
      expect(document.querySelectorAll("thead th")).toHaveLength(7);
    });

    it("公開の動画が無ければ、取り込みでなくログインへの入口を出す", async () => {
      fetchMock.mockImplementation(() =>
        Promise.resolve(json({ items: [], total: 0 } satisfies VideoPage)),
      );
      renderLibrary("/", "guest");
      expect(await screen.findByText("No videos are public")).toBeDefined();
      expect(screen.getByText("Sign in to see all videos.")).toBeDefined();
      expect(screen.queryByRole("link", { name: "Scan" })).toBeNull();
      const login = screen.getByRole("link", { name: "Sign in" });
      expect(login.getAttribute("href")).toBe("/login?next=%2F");
    });
  });
  describe("グループのカード（specs/017-folder-groups/ui-design.md「Group card」）", () => {
    const memberIds = Array.from({ length: 12 }, (_, i) => 101 + i);

    function seriesGroup(extra: Partial<LibraryGroup> = {}): LibraryGroup {
      return {
        folder: { rootId: 3, path: "series" },
        name: "series",
        videoCount: 12,
        watchedCount: 3,
        watchState: "inProgress",
        durationMs: 12 * 60_000,
        sizeBytes: 12 * 1024 * 1024,
        addedAt: "2026-09-02T00:00:00Z",
        previews: [101, 102, 103, 104].map((id) => ({
          videoId: id,
          thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
        })),
        openVideoId: 104,
        videoIds: memberIds,
        tags: [],
        ...extra,
      };
    }

    /** PlayerStub は再生画面の代わりに、開いた id と、元の一覧へ戻る操作を置く。 */
    function PlayerStub() {
      const location = useLocation();
      const navigate = useNavigate();
      const from = (location.state as { from?: string } | null)?.from ?? "";
      return (
        <>
          <p>{`再生画面 ${location.pathname}`}</p>
          <button type="button" onClick={() => void navigate(from)}>
            {`一覧へ戻る ${from}`}
          </button>
        </>
      );
    }

    function renderWithPlayer(initial = "/", audience: Audience = "owner") {
      return render(
        <MemoryRouter initialEntries={[initial]}>
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

    interface Server {
      group: LibraryGroup;
      tagRequests: { videoIds: number[] }[];
    }

    function installGroupList(group: LibraryGroup = seriesGroup()): Server {
      const server: Server = { group, tagRequests: [] };
      const tag = { id: 1, name: "旅行" };
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
        if (url.pathname === "/api/tags" && method === "GET") {
          return Promise.resolve(
            json({ items: [{ ...tag, synonyms: [], videoCount: 0 }] }),
          );
        }
        if (url.pathname === "/api/library") {
          return Promise.resolve(
            json({
              items: [
                { kind: "video", video: video(1) },
                { kind: "group", group: server.group },
                { kind: "video", video: video(2) },
              ],
              total: 3,
            }),
          );
        }
        if (url.pathname === "/api/library/ids") {
          return Promise.resolve(json({ ids: [1, ...memberIds, 2] }));
        }
        if (url.pathname === "/api/folders/3/group") {
          return Promise.resolve(json(server.group));
        }
        if (url.pathname === "/api/video-tags" && method === "POST") {
          const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
          server.tagRequests.push({ videoIds: body.videoIds });
          return Promise.resolve(json({ tag, applied: body.videoIds.length }));
        }
        if (url.pathname === "/api/video-tags/summary" && method === "POST") {
          const body = JSON.parse(String(init?.body)) as { videoIds: number[] };
          return Promise.resolve(json({ total: body.videoIds.length, items: [] }));
        }
        throw new Error(`unexpected request: ${url.toString()}`);
      });
      return server;
    }

    const ownerLabel = "series, group of 12 videos, 3 watched";

    it("グループはふつうの動画と同じ格子に1枚のカードで混ざり、件数はカードの枚数（受け入れ条件1・2）", async () => {
      installGroupList();
      renderLibrary();
      const link = await screen.findByRole("link", { name: ownerLabel });

      // 動画・グループ・動画が同じ並びに、区画や見出しなしで3枚並ぶ。
      const cards = document.querySelectorAll("article");
      expect(cards).toHaveLength(3);
      expect(cards[1]?.contains(link)).toBe(true);
      expect(screen.queryByRole("heading", { level: 2 })).toBeNull();
      // メンバーは1本ずつのカードにならない。
      expect(screen.queryByRole("link", { name: "ep01" })).toBeNull();
      expect(screen.getByRole("status").textContent).toBe("3 items");

      const card = cards[1] as HTMLElement;
      // 本数と長さはフォルダの絵柄の上に重ね、見終えた本数は数字では出さない。
      expect(within(card).getByText("12 videos")).toBeDefined();
      expect(within(card).queryByText(/\/ 12/)).toBeNull();
      expect(within(card).getByText("12:00")).toBeDefined();
      expect(
        card.querySelectorAll("[data-folder-art] [data-folder-preview]"),
      ).toHaveLength(4);
      const progress = within(card).getByRole("progressbar", {
        name: "Share of videos watched",
      });
      expect(progress.getAttribute("aria-valuenow")).toBe("25");
      expect(within(card).getByRole("heading", { level: 3 }).textContent).toBe("series");
    });

    it("支援技術向けの名前にグループであることと本数が入る。見始めていなければ視聴済みを足さない", async () => {
      installGroupList(
        seriesGroup({ watchedCount: 0, watchState: "unwatched", openVideoId: 101 }),
      );
      renderLibrary();
      const link = await screen.findByRole("link", {
        name: "series, group of 12 videos",
      });
      const card = link.closest("article") as HTMLElement;
      expect(within(card).getByText("12 videos")).toBeDefined();
      expect(within(card).queryByRole("progressbar")).toBeNull();
      expect(
        screen.getByRole("checkbox", { name: 'Select the group "series"' }),
      ).toBeDefined();
    });

    it("押すと開くメンバーの再生画面へ移り、戻る先は今の一覧（受け入れ条件14）", async () => {
      installGroupList();
      const user = userEvent.setup();
      renderWithPlayer("/?q=ser");
      await user.click(await screen.findByRole("link", { name: ownerLabel }));
      expect(await screen.findByText("再生画面 /videos/104")).toBeDefined();
      expect(screen.getByRole("button", { name: "一覧へ戻る /?q=ser" })).toBeDefined();
    });

    it("再生から戻ると、カードの視聴状態がメンバーの変化で変わる（受け入れ条件12）", async () => {
      const server = installGroupList();
      const user = userEvent.setup();
      renderWithPlayer();
      await user.click(await screen.findByRole("link", { name: ownerLabel }));
      await screen.findByText("再生画面 /videos/104");

      // 再生画面で 104 を見終えた。サーバーのグループも 4 本視聴済みになる。
      server.group = seriesGroup({ watchedCount: 4, openVideoId: 105 });
      act(() => {
        recordSavedProgress(
          104,
          { positionMs: 60_000, completed: true, updatedAt: "" },
          nextProgressSequence(),
        );
      });
      await user.click(screen.getByRole("button", { name: /^一覧へ戻る/ }));

      const link = await screen.findByRole("link", {
        name: "series, group of 12 videos, 4 watched",
      });
      const card = link.closest("article") as HTMLElement;
      expect(
        within(card)
          .getByRole("progressbar", { name: "Share of videos watched" })
          .getAttribute("aria-valuenow"),
      ).toBe("33");
      expect(link.getAttribute("href")).toBe("/videos/105");
      // 一覧は控えから戻し、グループだけを取り直す。
      const libraryRequests = listRequests(fetchMock);
      expect(libraryRequests).toHaveLength(1);
    });

    it("グループを選ぶと全メンバーが選択に入り、タグを付けると全メンバーに付く（受け入れ条件13）", async () => {
      const server = installGroupList();
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: ownerLabel });

      await user.click(
        screen.getByRole("checkbox", { name: 'Select the group "series"' }),
      );
      // 選択バーの本数はメンバーを数える。
      expect(screen.getByText("12 videos selected")).toBeDefined();
      const card = screen.getByRole("link", { name: ownerLabel }).closest("article");
      expect(
        card
          ?.querySelector('[data-slot="video-thumbnail"]')
          ?.hasAttribute("data-selected"),
      ).toBe(true);
      // 選んだ本数（12）が項目の数（3）を超えても、「すべて選択」は押せる。
      expect(
        (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false);

      await user.click(screen.getByRole("button", { name: "Add tag" }));
      const input = await screen.findByRole("combobox", { name: "Add tag" });
      await user.type(input, "旅行");
      await screen.findByRole("option", { name: /旅行/ });
      await user.keyboard("{Enter}");

      expect(await screen.findByText('Added "旅行" to 12 videos')).toBeDefined();
      expect(server.tagRequests).toHaveLength(1);
      expect([...(server.tagRequests[0]?.videoIds ?? [])].sort((a, b) => a - b)).toEqual(
        memberIds,
      );

      // 外すと全メンバーが選択から外れる。
      await user.click(
        screen.getByRole("checkbox", { name: 'Select the group "series"' }),
      );
      await waitFor(() =>
        expect(screen.queryByText(/^\d[\d,]* videos? selected$/)).toBeNull(),
      );
    });

    it("選択中にグループのカードを押すと、開かずに選択を切り替える", async () => {
      installGroupList();
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: ownerLabel });
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));

      await user.click(screen.getByRole("link", { name: ownerLabel }));
      expect(screen.getByText("13 videos selected")).toBeDefined();
      expect(screen.queryByText(/再生画面/)).toBeNull();
    });

    it("すべて選択は全メンバーを選び、選択がその応答と同じ集合の間だけ押せない", async () => {
      installGroupList();
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: ownerLabel });
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));

      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByText("14 videos selected")).toBeDefined();
      expect(
        (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement)
          .disabled,
      ).toBe(true);
      expect(
        (
          screen.getByRole("checkbox", {
            name: 'Select the group "series"',
          }) as HTMLButtonElement
        ).getAttribute("aria-checked"),
      ).toBe("true");

      // 1本外すと、また押せる。
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
      expect(screen.getByText("13 videos selected")).toBeDefined();
      expect(
        (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false);
    });

    it("選択が一度消えたら、同じ id を手で選び直しても「すべて選択」を押せる", async () => {
      installGroupList();
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: ownerLabel });
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("button", { name: "Select all" }));
      expect(await screen.findByText("14 videos selected")).toBeDefined();

      // Esc で選択を解除する（条件の変更と同じく、選択が消える）。
      await user.keyboard("{Escape}");
      await waitFor(() =>
        expect(screen.queryByText(/^\d[\d,]* videos? selected$/)).toBeNull(),
      );

      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
      await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
      await user.click(
        screen.getByRole("checkbox", { name: 'Select the group "series"' }),
      );
      expect(screen.getByText("14 videos selected")).toBeDefined();
      expect(
        (screen.getByRole("button", { name: "Select all" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false);
    });

    it("リスト表示では同じ列にグループの値を出す", async () => {
      localStorage.setItem(
        "vv.view.v2",
        JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
      );
      installGroupList();
      renderLibrary();
      const link = await screen.findByRole("link", { name: ownerLabel });
      const row = link.closest("tr") as HTMLElement;
      expect(link.getAttribute("href")).toBe("/videos/104");
      expect(within(row).getByText("12 videos")).toBeDefined();
      expect(within(row).getByText("3 / 12")).toBeDefined();
      expect(within(row).getByText("12:00")).toBeDefined();
      expect(within(row).getAllByRole("cell")).toHaveLength(9);
    });

    it("ゲストのグループのカードには視聴状態と見終えた本数を出さない", async () => {
      // ゲストの応答では watchedCount・watchState を省く（data-model.md §7）。
      const guest = seriesGroup();
      delete guest.watchedCount;
      delete guest.watchState;
      installGroupList(guest);
      renderLibrary("/", "guest");
      const link = await screen.findByRole("link", {
        name: "series, group of 12 videos",
      });
      const card = link.closest("article") as HTMLElement;
      expect(within(card).getByText("12 videos")).toBeDefined();
      expect(within(card).queryByText(/^\d+ \/ 12$/)).toBeNull();
      expect(within(card).queryByRole("progressbar")).toBeNull();
      expect(screen.queryByRole("checkbox")).toBeNull();
    });
  });
});

describe("LibraryPage の英語の画面（specs/023-english-i18n）", () => {
  const fetchMock = vi.fn<typeof fetch>();
  /** library は GET /api/library の応答を作る。テストごとに差し替える。 */
  let library: (url: URL) => Promise<Response>;

  const kyoto = video(1, {
    title: "京都の旅",
    tags: [
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
      { id: 2, name: "京都", manual: false, fromFolder: true, tentative: false },
    ],
    progress: { positionMs: 15_000, completed: false, updatedAt: "" },
  });
  const publicVideo = video(2, {
    public: true,
    playable: false,
    probeState: "failed",
    thumbnailState: "failed",
    thumbnailUrl: undefined,
  });
  const group: LibraryGroup = {
    folder: { rootId: 3, path: "連続もの" },
    name: "連続もの",
    videoCount: 2,
    watchedCount: 1,
    watchState: "inProgress",
    durationMs: 5 * 60_000,
    sizeBytes: 5 * 1024 * 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: [{ videoId: 11, thumbnailUrl: "/api/videos/11/thumbnail" }],
    openVideoId: 11,
    videoIds: [11, 12],
    tags: [],
  };
  /** 画面に出る利用者のデータ（名前）と、ロケールに依らない書式（時間・容量）。 */
  const userData = [
    "京都の旅",
    "動画 2",
    "連続もの",
    "旅行",
    "京都",
    formatDuration(kyoto.durationMs),
    formatDuration(publicVideo.durationMs),
    formatDuration(group.durationMs),
    formatBytes(kyoto.sizeBytes),
    formatBytes(publicVideo.sizeBytes),
    formatBytes(group.sizeBytes),
    "1080p",
  ];

  function renderPage(initial = "/", audience: Audience = "owner") {
    return render(
      <MemoryRouter initialEntries={[initial]}>
        <TooltipProvider>
          <ToastProvider>
            <AudienceProvider audience={audience}>
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

  function page(items: unknown[], total = items.length): Promise<Response> {
    return Promise.resolve(json({ items, total }));
  }

  beforeEach(() => {
    __resetTagsForTest();
    clearListSnapshot();
    window.localStorage.clear();
    vi.stubGlobal("fetch", libraryFetch(fetchMock));
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    library = () =>
      page([
        { kind: "video", video: kyoto },
        { kind: "group", group },
        { kind: "video", video: publicVideo },
      ]);
    fetchMock.mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
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
            items: [
              { id: 1, name: "旅行", synonyms: [], videoCount: 1 },
              { id: 2, name: "京都", synonyms: [], videoCount: 1 },
            ],
          }),
        );
      }
      if (url.pathname === "/api/library") return library(url);
      throw new Error(`unexpected request: ${url.toString()}`);
    });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  it("件数は 1 件と複数件で単数・複数を分ける", async () => {
    library = () => page([{ kind: "video", video: kyoto }]);
    const { unmount } = renderPage();
    await screen.findByRole("link", { name: "京都の旅" });
    expect(screen.getByRole("status").textContent).toBe("1 item");
    unmount();

    library = () =>
      page([
        { kind: "video", video: kyoto },
        { kind: "video", video: publicVideo },
      ]);
    renderPage();
    await screen.findByRole("link", { name: "動画 2" });
    expect(screen.getByRole("status").textContent).toBe("2 items");
  });

  it("日本語の名前の動画とタグを元の名前のまま出し、その名前で検索できる", async () => {
    const user = userEvent.setup();
    library = (url) =>
      url.searchParams.get("query") === "京都"
        ? page([{ kind: "video", video: kyoto }])
        : page([
            { kind: "video", video: kyoto },
            { kind: "video", video: publicVideo },
          ]);
    renderPage();
    await screen.findByRole("link", { name: "動画 2" });
    expect(screen.getByRole("heading", { level: 3, name: "京都の旅" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Filter by 旅行" })).toBeDefined();
    expect(
      screen.getByRole("button", { name: "Filter by 京都 (from the folder name)" }),
    ).toBeDefined();

    await user.type(screen.getByRole("searchbox", { name: "Search videos" }), "京都");
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "動画 2" })).toBeNull(),
    );
    expect(await screen.findByRole("link", { name: "京都の旅" })).toBeDefined();
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("1 item"));
    expect(listRequests(fetchMock).at(-1)?.searchParams.get("query")).toBe("京都");
  });

  it("検索語が長すぎる失敗は、API の limit を埋め込んだ文で出す", async () => {
    library = () =>
      Promise.resolve(
        json(
          {
            code: "invalid_request",
            reason: "search_too_long",
            limit: 100,
            message: "query must be at most 100 characters",
          },
          400,
        ),
      );
    renderPage("/?q=abc");
    expect(
      await screen.findByText("Use a search of 100 characters or fewer."),
    ).toBeDefined();
    expect(screen.getByRole("heading", { name: "Couldn't load the list" })).toBeDefined();
  });

  it("疑似ロケールで、一覧・選択・ツールバーの文言がすべてカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: /京都の旅/ });
    expectCatalogTextOnly(document.body, userData);

    await user.click(screen.getByRole("checkbox", { name: /京都の旅/ }));
    await screen.findByRole("region", { name: /Selection actions/ });
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "⟦Filter⟧" }));
    await screen.findByRole("dialog");
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: /How to search/ }));
    await screen.findByRole("dialog");
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: /View and sort/ }));
    const dialog = await screen.findByRole("dialog");
    expectCatalogTextOnly(document.body, userData);
    await user.click(within(dialog).getAllByRole("radio", { name: /List/ })[0]!);
    await screen.findByRole("table");
    expectCatalogTextOnly(document.body, userData);
  });

  it("疑似ロケールで、タグの絞り込み中と一致なしの文言がカタログから出る", async () => {
    enablePseudoLocale();
    library = () => page([]);
    renderPage("/?tag=1&q=zzz");
    await screen.findByRole("heading", { name: /No videos match/ });
    await screen.findByRole("button", { name: /旅行/ });
    expectCatalogTextOnly(document.body, [...userData, "zzz"]);
  });

  it("疑似ロケールで、空・ゲストの空・読み込み中・失敗の文言がカタログから出る", async () => {
    enablePseudoLocale();
    library = () => page([]);
    const empty = renderPage();
    await screen.findByRole("heading", { name: /No videos yet/ });
    expectCatalogTextOnly(document.body);
    empty.unmount();

    const guest = renderPage("/", "guest");
    await screen.findByRole("heading", { name: /No videos are public/ });
    expectCatalogTextOnly(document.body);
    guest.unmount();

    library = () => new Promise<Response>(() => undefined);
    const loading = renderPage();
    await screen.findByText(/Loading/);
    expectCatalogTextOnly(document.body);
    loading.unmount();

    library = () =>
      Promise.resolve(json({ code: "internal", message: "internal error" }, 500));
    renderPage();
    await screen.findByRole("heading", { name: /Couldn't load the list/ });
    expectCatalogTextOnly(document.body);
  });
});

describe("LibraryPage の束ねる操作（specs/030-video-versions/ui-design.md「Selection bar」）", () => {
  const fetchMock = vi.fn<typeof fetch>();
  /** 一覧に出る動画の id。束ねると代表以外が消える。 */
  let listed: number[];
  let bundleRequests: { videoIds: number[]; representativeId: number }[];
  const group: LibraryGroup = {
    folder: { rootId: 3, path: "連続もの" },
    name: "連続もの",
    videoCount: 2,
    watchedCount: 0,
    watchState: "unwatched",
    durationMs: 5 * 60_000,
    sizeBytes: 5 * 1024 * 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: [{ videoId: 11, thumbnailUrl: "/api/videos/11/thumbnail" }],
    openVideoId: 11,
    videoIds: [11, 12],
    tags: [],
  };
  let withGroup: boolean;

  beforeEach(() => {
    __resetTagsForTest();
    clearListSnapshot();
    listed = [1, 2, 3];
    bundleRequests = [];
    withGroup = false;
    vi.stubGlobal("fetch", libraryFetch(fetchMock));
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
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
      if (url.pathname === "/api/library") {
        const items: unknown[] = listed.map((id) => ({
          kind: "video",
          video: video(id),
        }));
        if (withGroup) items.push({ kind: "group", group });
        return Promise.resolve(json({ items, total: items.length }));
      }
      const detail = /^\/api\/videos\/(\d+)$/.exec(url.pathname);
      if (detail !== null) return Promise.resolve(json(video(Number(detail[1]))));
      if (url.pathname === "/api/video-bundles" && method === "POST") {
        const body = JSON.parse(String(init?.body)) as {
          videoIds: number[];
          representativeId: number;
        };
        bundleRequests.push(body);
        listed = listed.filter(
          (id) => id === body.representativeId || !body.videoIds.includes(id),
        );
        if (body.videoIds.some((id) => group.videoIds.includes(id))) withGroup = false;
        return Promise.resolve(
          json({
            representativeId: body.representativeId,
            items: [
              video(body.representativeId),
              ...body.videoIds
                .filter((id) => id !== body.representativeId)
                .map((id) => video(id)),
            ],
          }),
        );
      }
      throw new Error(`unexpected request: ${method} ${url.toString()}`);
    });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("2 本を選んで代表を選び束ねると、一覧に代表の 1 件だけが残り選択が解ける", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    // 1 本の選択では出さない。
    expect(screen.queryByRole("button", { name: "Bundle as versions" })).toBeNull();
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
    await user.click(screen.getByRole("button", { name: "Bundle as versions" }));

    const dialog = screen.getByRole("dialog", { name: "Bundle as versions" });
    await user.click(await within(dialog).findByRole("radio", { name: /動画 2/ }));
    await user.click(within(dialog).getByRole("button", { name: "Bundle" }));

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "動画 1" })).toBeNull(),
    );
    expect(bundleRequests).toEqual([{ videoIds: [1, 2], representativeId: 2 }]);
    expect(screen.getByRole("link", { name: "動画 2" })).toBeDefined();
    expect(screen.getByRole("link", { name: "動画 3" })).toBeDefined();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
    expect(
      await screen.findByText('Bundled 2 videos as versions of "動画 2"'),
    ).toBeDefined();
    // バーは消えるので、フォーカスは一覧の最初のカードへ移る。
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("link", { name: "動画 2" })),
    );
  });

  it("グループの選択はメンバーに広げて送る", async () => {
    const user = userEvent.setup();
    withGroup = true;
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "連続もの"' }),
    );
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("button", { name: "Bundle as versions" }));

    const dialog = screen.getByRole("dialog", { name: "Bundle as versions" });
    const radios = await within(dialog).findAllByRole("radio");
    expect(radios).toHaveLength(3);
    await user.click(within(dialog).getByRole("radio", { name: /^動画 1,/ }));
    await user.click(within(dialog).getByRole("button", { name: "Bundle" }));

    await waitFor(() => expect(bundleRequests).toHaveLength(1));
    expect([...bundleRequests[0]!.videoIds].sort((a, b) => a - b)).toEqual([1, 11, 12]);
    expect(bundleRequests[0]!.representativeId).toBe(1);
  });

  it("窓の Esc は窓だけを閉じ、選択を残す", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 3"' }));
    await user.click(screen.getByRole("button", { name: "Bundle as versions" }));
    await screen.findAllByRole("radio");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("region", { name: "Selection actions" })).toBeDefined();
    expect(screen.getByText("2 videos selected")).toBeDefined();
    expect(bundleRequests).toEqual([]);
  });

  it("送る間にフォーカスが body に落ちても、Esc で選択を解除せず窓を残す", async () => {
    const user = userEvent.setup();
    const base = fetchMock.getMockImplementation()!;
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    fetchMock.mockImplementation(async (input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/video-bundles") await held;
      return base(input, init);
    });
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
    await user.click(screen.getByRole("button", { name: "Bundle as versions" }));

    const dialog = screen.getByRole("dialog", { name: "Bundle as versions" });
    await user.click(await within(dialog).findByRole("radio", { name: /動画 2/ }));
    await user.click(within(dialog).getByRole("button", { name: "Bundle" }));
    await waitFor(() =>
      expect(
        (within(dialog).getByRole("button", { name: "Bundle" }) as HTMLButtonElement)
          .disabled,
      ).toBe(true),
    );
    // ブラウザは disabled になったボタンからフォーカスを外し body に落とすので、Esc の
    // 対象は body になる（jsdom はフォーカスを残すため、対象を body にして送る）。
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(screen.getByRole("dialog", { name: "Bundle as versions" })).toBeDefined();
    expect(screen.getByText("2 videos selected")).toBeDefined();

    release();
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "動画 1" })).toBeNull(),
    );
    expect(bundleRequests).toEqual([{ videoIds: [1, 2], representativeId: 2 }]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      await screen.findByText('Bundled 2 videos as versions of "動画 2"'),
    ).toBeDefined();
  });
});
