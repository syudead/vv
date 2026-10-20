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
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { formatBytes, formatDuration } from "../lib/format";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
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

function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <span data-testid="location">{location.search}</span>
      <span data-testid="pathname">{location.pathname}</span>
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

/** Adapt video-only fixtures to mixed library items while preserving existing discriminated items. */
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

/** Return empty tag kinds here; LibraryTagKindFilter.test.tsx covers kind filtering. */
function libraryFetch(fetchMock: ReturnType<typeof vi.fn<typeof fetch>>): typeof fetch {
  return (input, init) => {
    const path = new URL(String(input), "http://localhost").pathname;
    if (path === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    const result = fetchMock(input, init);
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

describe("LibraryPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
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

  it("ホームは一覧だけを出し、見出しと件数を出さない", async () => {
    renderLibrary();
    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
    expect(screen.queryByRole("heading", { level: 2 })).toBeNull();
    expect(screen.queryByRole("status", { name: "Search results" })).toBeNull();
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("25");
  });

  it("検索語は URL から読み、結果件数だけを出す", async () => {
    renderLibrary("/?q=abc");
    await waitFor(() => {
      expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
        "3 items",
      );
    });
    expect(screen.getByRole("heading", { level: 2 }).textContent).toContain(
      "Search results",
    );
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

  it("空なら所有者をオンボードへ案内する", async () => {
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
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Set up library" }));
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            String(input) === "/api/onboarding" &&
            init?.method === "PUT" &&
            init.body === JSON.stringify({ step: "folders" }),
        ),
      ).toBe(true),
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
    expect(screen.queryByRole("status", { name: "Search results" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(resolveRetry).toBeDefined());
    expect(screen.getByRole("status", { name: "Loading…" })).toBeDefined();
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
    await screen.findByRole("link", { name: "動画 1" });

    await user.type(screen.getByRole("searchbox", { name: "Search videos" }), "broken");

    expect(await screen.findByText("Something went wrong on the server.")).toBeDefined();
    expect(screen.queryByRole("status", { name: "Search results" })).toBeNull();
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
    expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
      "250 items",
    );
    const requests = listRequests(fetchMock);
    expect(requests.at(-1)?.searchParams.get("watch")).toBe("unwatched");
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
    expect(
      (screen.getByRole("searchbox", { name: "Search videos" }) as HTMLInputElement)
        .value,
    ).toBe("京都");
    expect(screen.getByRole("button", { name: "Filter (2 applied)" })).toBeDefined();
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

  describe("お気に入りのみと「Date favorited」", () => {
    it("「Favorites only」を入れると favorite=true で読み、URL に fav=1 が付き、ボタンの数字に数え、「Clear filters」で外れる（要件 8）", async () => {
      const user = userEvent.setup();
      renderLibrary("/?sort=titleAsc");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter" }));
      const filter = await screen.findByRole("dialog");
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

  it("未視聴で絞った一覧から動画を開き、見終えて戻っても、控えから同じ並びと件数で戻し読み直さない", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      return Promise.resolve(
        json({ items: [video(3), video(2), video(1)], total: 3 } satisfies VideoPage),
      );
    });
    const view = renderLibrary("/?watch=unwatched&sort=titleDesc");
    await screen.findByRole("link", { name: "動画 2" });
    const titles = () =>
      Array.from(document.querySelectorAll("[data-video-id] h3"), (h) => h.textContent);
    const before = titles();
    const requested = listRequests(fetchMock).length;

    fireEvent.click(screen.getByRole("link", { name: "動画 2" }));
    await screen.findByText("再生画面");
    act(() =>
      recordSavedProgress(
        2,
        { positionMs: 1_500, completed: true, updatedAt: "" },
        nextProgressSequence(),
      ),
    );
    view.unmount();

    renderLibrary("/?watch=unwatched&sort=titleDesc");
    await screen.findByRole("link", { name: "動画 2" });
    expect(titles()).toEqual(before);
    expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
      "3 items",
    );
    expect(listRequests(fetchMock)).toHaveLength(requested);
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

    expect(takeListSnapshot({ query: "" })).toBeUndefined();

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

    it("keeps /?tag=3 on home and filters the list by tag 3", async () => {
      const tag = {
        id: 3,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      };
      const requested: number[][] = [];
      installTagAwareList([tag], (tags) => {
        requested.push(tags);
        return { items: [video(1, { tags: [tag] })], total: 1 };
      });
      renderLibrary("/?tag=3");

      await screen.findByRole("link", { name: "動画 1" });
      expect(screen.getByTestId("pathname").textContent).toBe("/");
      expect(screen.getByTestId("location").textContent).toContain("tag=3");
      expect(requested.at(-1)).toEqual([3]);
      expect(
        within(screen.getByRole("list", { name: "Tags in the filter" })).getByText(
          "旅行",
        ),
      ).toBeDefined();
    });

    it("/?tag=1&sort=random は seed を補い、tag=1 を残す", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      renderLibrary("/?tag=1&sort=random");
      await screen.findByRole("link", { name: "動画 1" });

      const location = () => screen.getByTestId("location").textContent ?? "";
      await waitFor(() => expect(location()).toMatch(/seed=\d+/));
      expect(location()).toMatch(/tag=1/);
      expect(location()).toMatch(/sort=random/);
    });

    it("タグの絞り込みを変えると選択を解除する", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
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
        kind: null,
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
        kind: null,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?q=abc&watch=unwatched");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "Filter by 旅行" }));

      const location = () => screen.getByTestId("location").textContent ?? "";
      await waitFor(() => expect(location()).toContain("tag=1"));
      expect(location()).toContain("q=abc");
      expect(location()).toContain("watch=unwatched");
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
        kind: null,
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
        kind: null,
      };
      const tagB = {
        id: 2,
        name: "2024",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
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
      const location = () => screen.getByTestId("location").textContent ?? "";
      await waitFor(() => expect(location()).not.toMatch(/tag=2(&|$)/));
      expect(location()).toContain("tag=1");
      expect(location()).toContain("q=abc");
    });

    it("missingTagIds を受けたら伝えて、タグの一覧を取り直し、一覧も取り直して URL から取り除く（N1）", async () => {
      const tagA = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
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

      await waitFor(() => {
        const tagRequests = fetchMock.mock.calls.filter((call) =>
          new URL(String(call[0]), "http://localhost").pathname.startsWith("/api/tags"),
        );
        expect(tagRequests.length).toBeGreaterThanOrEqual(2);
      });
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
      installTagAwareList([], () => ({ items: [video(1)], total: 1 }));
      // Set sort explicitly so earlier preference changes cannot alter the snapshot key.
      renderLibrary("/?tag=1&sort=addedDesc");

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
        kind: null,
      };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));

      renderLibrary("/?tag=1");

      expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
      expect(screen.queryByRole("link", { name: "控えの動画" })).toBeNull();
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
        kind: null,
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

  describe("選択バーのタグ一括操作（issue 270）", () => {
    function installSelectionAwareList(options: {
      tags?: { id: number; name: string }[];
      total?: number;
    }) {
      const tags = options.tags ?? [];
      const total = options.total ?? 3;
      const attached = new Map<number, Set<number>>();
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
                    kind: null,
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
    }

    it("選択バーでタグを付けると、読み込み済みのカードにすぐ出て、選択は残る", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
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
      expect(screen.getByText("2 videos selected")).toBeDefined();
      // Count visible chips only, excluding hidden overflow-measurement duplicates.
      const card = screen.getByText("動画 1").closest("article");
      expect(card).not.toBeNull();
      const tagList = within(card as HTMLElement).getByRole("list", { name: "Tags" });
      expect(within(tagList).getByText("旅行")).toBeDefined();
    });

    it("Shift を押してチェックすると、前にチェックした動画との間を同じ状態にする", async () => {
      installSelectionAwareList({});
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });
      const check = (n: number) =>
        screen.getByRole("checkbox", { name: `Select "動画 ${String(n)}"` });

      await user.click(check(1));
      await user.keyboard("{Shift>}");
      await user.click(check(3));
      expect(screen.getByText("3 videos selected")).toBeDefined();
      expect(check(2).getAttribute("aria-checked")).toBe("true");

      await user.click(check(2));
      await user.keyboard("{/Shift}");
      expect(screen.getByText("1 video selected")).toBeDefined();
      expect(check(1).getAttribute("aria-checked")).toBe("true");
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
      expect(screen.getByText("1 video selected")).toBeDefined();
    });

    // （docs/design-docs/library-ui.md）。
    it("一括で外したタグが今の絞り込みに含まれるとき、選択を解除して一覧を取り直す（Devin の指摘4）", async () => {
      const tag = {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
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

      await waitFor(() =>
        expect(screen.queryByText(/^\d[\d,]* videos? selected$/)).toBeNull(),
      );
      expect(await screen.findByText("No videos match these conditions")).toBeDefined();
    });
  });
  describe("ゲスト", () => {
    function guestPage(): VideoPage {
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
      //
      expect(FakeEventSource.instances).toHaveLength(1);
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
  describe("グループのカード", () => {
    const memberIds = Array.from({ length: 12 }, (_, i) => 101 + i);

    function seriesGroup(extra: Partial<LibraryGroup> = {}): LibraryGroup {
      return {
        id: 7,
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
                    <Route path="/groups/:id" element={<PlayerStub />} />
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
        if (url.pathname === "/api/groups/7") {
          return Promise.resolve(json({ group: server.group, trail: [], items: [] }));
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
      renderLibrary("/?q=s");
      const link = await screen.findByRole("link", { name: ownerLabel });

      const cards = document.querySelectorAll("article");
      expect(cards).toHaveLength(3);
      expect(cards[1]?.contains(link)).toBe(true);
      expect(screen.queryByRole("link", { name: "ep01" })).toBeNull();
      expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
        "3 items",
      );

      const card = cards[1] as HTMLElement;
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

    it("押すとそのグループのページへ移り、戻る先は今の一覧", async () => {
      installGroupList();
      const user = userEvent.setup();
      renderWithPlayer("/?q=ser");
      await user.click(await screen.findByRole("link", { name: ownerLabel }));
      expect(await screen.findByText("再生画面 /groups/7")).toBeDefined();
      expect(screen.getByRole("button", { name: "一覧へ戻る /?q=ser" })).toBeDefined();
    });

    it("グループのページから戻ると、カードの視聴状態がメンバーの変化で変わる（受け入れ条件12）", async () => {
      const server = installGroupList();
      const user = userEvent.setup();
      renderWithPlayer();
      await user.click(await screen.findByRole("link", { name: ownerLabel }));
      await screen.findByText("再生画面 /groups/7");

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
      expect(link.getAttribute("href")).toBe("/groups/7");
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
      expect(screen.getByText("12 videos selected")).toBeDefined();
      const card = screen.getByRole("link", { name: ownerLabel }).closest("article");
      expect(
        card
          ?.querySelector('[data-slot="video-thumbnail"]')
          ?.hasAttribute("data-selected"),
      ).toBe(true);

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

    it("リスト表示では同じ列にグループの値を出す", async () => {
      localStorage.setItem(
        "vv.view.v2",
        JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
      );
      installGroupList();
      renderLibrary();
      const link = await screen.findByRole("link", { name: ownerLabel });
      const row = link.closest("tr") as HTMLElement;
      expect(link.getAttribute("href")).toBe("/groups/7");
      expect(within(row).getByText("12 videos")).toBeDefined();
      expect(within(row).getByText("3 / 12")).toBeDefined();
      expect(within(row).getByText("12:00")).toBeDefined();
      expect(within(row).getAllByRole("cell")).toHaveLength(11);
    });

    it("ゲストのグループのカードには視聴状態と見終えた本数を出さない", async () => {
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

describe("LibraryPage の英語の画面", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let library: (url: URL) => Promise<Response>;

  const kyoto = video(1, {
    title: "京都の旅",
    tags: [
      {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
      {
        id: 2,
        name: "京都",
        manual: false,
        fromFolder: true,
        tentative: false,
        kind: null,
      },
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
    id: 7,
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
    __resetTagKindsForTest();
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
    const { unmount } = renderPage("/?q=a");
    await screen.findByRole("link", { name: "京都の旅" });
    expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
      "1 item",
    );
    unmount();

    library = () =>
      page([
        { kind: "video", video: kyoto },
        { kind: "video", video: publicVideo },
      ]);
    renderPage("/?q=a");
    await screen.findByRole("link", { name: "動画 2" });
    expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
      "2 items",
    );
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
    await waitFor(() =>
      expect(screen.getByRole("status", { name: "Search results" }).textContent).toBe(
        "1 item",
      ),
    );
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
    await screen.findByRole("status", { name: /Loading/ });
    expectCatalogTextOnly(document.body);
    loading.unmount();

    library = () =>
      Promise.resolve(json({ code: "internal", message: "internal error" }, 500));
    renderPage();
    await screen.findByRole("heading", { name: /Couldn't load the list/ });
    expectCatalogTextOnly(document.body);
  });
});

describe("LibraryPage の束ねる操作", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let listed: number[];
  let bundleRequests: { videoIds: number[]; representativeId: number }[];
  const group: LibraryGroup = {
    id: 7,
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
    __resetTagKindsForTest();
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
    // Browsers blur disabled buttons to body; jsdom retains focus, so dispatch Escape on body explicitly.
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
