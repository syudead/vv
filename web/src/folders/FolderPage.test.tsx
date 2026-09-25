import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";

import type {
  FolderListing,
  FolderSummary,
  RootFolderListing,
  Video,
  VideoPage,
} from "../api/client";
import { clearListSnapshot } from "../api/listSnapshot";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import FolderPage from "./FolderPage";

function summary(extra: Partial<FolderSummary>): FolderSummary {
  return {
    rootId: 3,
    path: "",
    name: "movies",
    rootPath: "/a/movies",
    videoCount: 0,
    folderCount: 0,
    previews: [],
    ...extra,
  };
}

function previews(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    videoId: index + 1,
    thumbnailUrl: `/api/videos/${String(index + 1)}/thumbnail?v=x`,
  }));
}

function video(id: number, title: string, extra: Partial<Video> = {}): Video {
  return {
    id,
    title,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
    ...extra,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const roots: RootFolderListing = {
  folders: [
    summary({ rootId: 3, rootPath: "/a/movies", videoCount: 0, folderCount: 9 }),
    summary({ rootId: 7, rootPath: "/b/movies", videoCount: 1, previews: previews(1) }),
  ],
};

const folderA: FolderListing = {
  folder: summary({ path: "A", name: "A", videoCount: 1, folderCount: 1 }),
  folders: [
    summary({
      path: "A/B",
      name: "B",
      videoCount: 1,
      folderCount: 1,
      previews: previews(1),
    }),
  ],
};

const folderRoot: FolderListing = {
  folder: summary({ folderCount: 3 }),
  folders: [
    summary({ path: "five", name: "five", videoCount: 6, previews: previews(4) }),
    summary({ path: "only-deeper", name: "only-deeper", folderCount: 1 }),
    summary({ path: "100% #1 日本語", name: "100% #1 日本語", videoCount: 1 }),
  ],
};

/** Player は再生画面の代わりで、「戻る」で履歴を1つ戻る。 */
function Player() {
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => void navigate(-1)}>
      戻る
    </button>
  );
}

/** LocationProbe は今の URL を見せ、履歴を1つ戻る操作を置く（戻る/進むの確認）。 */
function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <span data-testid="location">{`${location.pathname}${location.search}`}</span>
      <button type="button" onClick={() => void navigate(-1)}>
        テストで戻る
      </button>
    </>
  );
}

/** LibraryProbe はライブラリの代わりで、移ってきた URL を見せる。 */
function LibraryProbe() {
  const location = useLocation();
  return (
    <span data-testid="library-location">{`${location.pathname}${location.search}`}</span>
  );
}

function renderFolders(initial: string) {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <ScanProvider>
            <Routes>
              <Route
                path="/folders/*"
                element={
                  <>
                    <FolderPage />
                    <LocationProbe />
                  </>
                }
              />
              <Route path="/videos/:id" element={<Player />} />
              <Route path="/" element={<LibraryProbe />} />
            </Routes>
          </ScanProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("FolderPage", () => {
  const fetchMock = vi.fn<typeof fetch>();
  const requests: string[] = [];

  beforeEach(() => {
    requests.length = 0;
    vi.stubGlobal("fetch", fetchMock);
    installFakeEventSource();
    vi.stubGlobal("scrollTo", vi.fn());
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      requests.push(url);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/folders") return Promise.resolve(json(roots));
      if (url === "/api/folders/3?path=A") return Promise.resolve(json(folderA));
      if (url === "/api/folders/3") return Promise.resolve(json(folderRoot));
      if (url.startsWith("/api/folders/3/videos?")) {
        const params = new URL(url, "http://localhost").searchParams;
        if (params.get("path") !== "A")
          return Promise.resolve(json({ items: [], total: 0 }));
        if (params.get("query") === "京都") {
          const page: VideoPage = {
            items: [
              video(1, "x", { folder: { rootId: 3, path: "A" } }),
              video(2, "y", { folder: { rootId: 3, path: "A/B" } }),
            ],
            total: 2,
          };
          return Promise.resolve(json(page));
        }
        // 検索中にフォルダが無くなった状況を模す（listing は 200 のまま、配下の
        // 検索だけが 404 を返す）。
        if (params.get("query") === "gone") {
          return Promise.resolve(
            json({ code: "not_found", message: "そのフォルダは見つかりません" }, 404),
          );
        }
        if (params.get("query") === "broken") {
          return Promise.resolve(
            json({ code: "internal", message: "壊れています" }, 500),
          );
        }
        if (params.has("query")) return Promise.resolve(json({ items: [], total: 0 }));
        if (params.get("watch") === "unwatched") {
          const page: VideoPage = { items: [video(1, "x")], total: 1 };
          return Promise.resolve(json(page));
        }
        if (params.get("watch") === "watched") {
          return Promise.resolve(json({ items: [], total: 0 }));
        }
        // 絞り込みなし（直下）は常に x の1件。
        const page: VideoPage = { items: [video(1, "x")], total: 1 };
        return Promise.resolve(json(page));
      }
      if (url.startsWith("/api/videos?")) {
        const params = new URL(url, "http://localhost").searchParams;
        if (params.get("query") === "京都") {
          const page: VideoPage = {
            items: [
              video(1, "x", { folder: { rootId: 3, path: "A" } }),
              video(2, "y", { folder: { rootId: 3, path: "A/B" } }),
              video(4, "z", { folder: { rootId: 7, path: "" } }),
            ],
            total: 3,
          };
          return Promise.resolve(json(page));
        }
        return Promise.resolve(json({ items: [], total: 0 }));
      }
      // 準備中の項目は1件ずつ取り直される。実際のサーバーと同じく、その動画を返す。
      const single = /^\/api\/videos\/(\d+)$/.exec(url);
      if (single !== null) return Promise.resolve(json(video(Number(single[1]), "x")));
      return Promise.resolve(
        json({ code: "not_found", message: "そのフォルダは見つかりません" }, 404),
      );
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    clearListSnapshot();
  });

  it("動画のカードにタグを出し、押すとそのタグで絞ったライブラリへ移る（issue 308）", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/folders/3?path=A") return Promise.resolve(json(folderA));
      const page: VideoPage = {
        items: [video(1, "x", { tags: [{ id: 5, name: "旅行" }] })],
        total: 1,
      };
      return Promise.resolve(json(page));
    });
    renderFolders("/folders/3/A");
    const tag = await screen.findByRole("button", { name: "旅行で絞り込む" });
    await userEvent.setup().click(tag);
    expect(screen.getByTestId("library-location").textContent).toBe("/?tag=5");
  });

  it("ホバープレビューは同時に 1 件だけ再生する（issue 308）", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/folders/3?path=A") return Promise.resolve(json(folderA));
      const ready = { previewState: "done" as const };
      const page: VideoPage = {
        items: [
          video(1, "x", { ...ready, previewUrl: "/p/1" }),
          video(2, "y", { ...ready, previewUrl: "/p/2" }),
        ],
        total: 2,
      };
      return Promise.resolve(json(page));
    });
    const hover = async (title: string) => {
      const card = (await screen.findByRole("link", { name: title })).closest("article");
      if (card === null) throw new Error("card not found");
      fireEvent.pointerEnter(card, { pointerType: "mouse" });
      await act(() => new Promise((resolve) => setTimeout(resolve, 450)));
    };
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
    renderFolders("/folders/3/A");
    await screen.findByRole("link", { name: "x" });
    // 読み込み直後の描画と効果を済ませてから触れる。
    await act(() => new Promise((resolve) => setTimeout(resolve, 50)));
    await hover("x");
    expect(
      Array.from(document.querySelectorAll("video"), (v) => v.getAttribute("src")),
    ).toEqual(["/p/1"]);
    await hover("y");
    expect(
      Array.from(document.querySelectorAll("video"), (v) => v.getAttribute("src")),
    ).toEqual(["/p/2"]);
  });

  it("最上位に登録フォルダをパス付きで並べる", async () => {
    renderFolders("/folders");
    const a = await screen.findByRole("link", {
      name: "movies、動画 0 本、フォルダ 9 件、/a/movies",
    });
    const b = screen.getByRole("link", {
      name: "movies、動画 1 本、フォルダ 0 件、/b/movies",
    });
    expect(a.getAttribute("href")).toBe("/folders/3");
    expect(b.getAttribute("href")).toBe("/folders/7");
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe(
      "メディアフォルダ 2",
    );
  });

  it("直下の子フォルダと直下の動画を、子フォルダを先にして出す", async () => {
    renderFolders("/folders/3/A");
    const child = await screen.findByRole("link", {
      name: "B、動画 1 本、フォルダ 1 件",
    });
    const movie = await screen.findByRole("link", { name: "x" });
    expect(child.getAttribute("href")).toBe("/folders/3/A/B");
    expect(movie.getAttribute("href")).toBe("/videos/1");
    expect(
      child.compareDocumentPosition(movie) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // フォルダ画面は選択を持たない。
    expect(screen.queryByRole("checkbox")).toBeNull();

    const headings = screen
      .getAllByRole("heading", { level: 2 })
      .map((h) => h.textContent);
    expect(headings).toEqual(["フォルダ 1", "動画 1"]);
  });

  it("パンくずに現在地までの段を出し、現在地はリンクにしない", async () => {
    renderFolders("/folders/3/A");
    const nav = await screen.findByRole("navigation", { name: "パンくず" });
    await within(nav).findByRole("link", { name: "movies" });
    const links = within(nav)
      .getAllByRole("link")
      .map((link) => [link.textContent, link.getAttribute("href")]);
    expect(links).toEqual([
      ["フォルダ", "/folders"],
      ["movies", "/folders/3"],
    ]);
    const current = within(nav).getByText("A");
    expect(current.getAttribute("aria-current")).toBe("page");
  });

  it("プレビューは4件までで、サムネイルが無いフォルダは外形だけを描く", async () => {
    const { container } = renderFolders("/folders/3");
    await screen.findByRole("link", { name: "five、動画 6 本、フォルダ 0 件" });
    const five = container.querySelector('[data-folder-path="five"]');
    const deeper = container.querySelector('[data-folder-path="only-deeper"]');
    expect(five?.querySelectorAll("[data-folder-preview]").length).toBe(4);
    expect(five?.querySelectorAll("img").length).toBe(4);
    expect(deeper?.querySelectorAll("img").length).toBe(0);
    expect(deeper?.textContent).toContain("動画 0 本");
  });

  it("特殊な文字を含む名前のフォルダへ、段を1回だけ符号化したリンクを張る", async () => {
    renderFolders("/folders/3");
    const link = await screen.findByRole("link", {
      name: "100% #1 日本語、動画 1 本、フォルダ 0 件",
    });
    expect(link.getAttribute("href")).toBe(
      `/folders/3/${encodeURIComponent("100% #1 日本語")}`,
    );
  });

  it("並び順を変えると動画だけを読み直し、子フォルダは読み直さない", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A");
    await screen.findByRole("link", { name: "x" });
    const before = requests.filter((url) => url === "/api/folders/3?path=A").length;

    await user.click(screen.getByRole("button", { name: "並び順: 追加日" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "題名" }));

    await waitFor(() =>
      expect(requests.some((url) => url.includes("videos?path=A&sort=titleAsc"))).toBe(
        true,
      ),
    );
    expect(requests.filter((url) => url === "/api/folders/3?path=A").length).toBe(before);
    expect(
      screen.getByRole("link", { name: "B、動画 1 本、フォルダ 1 件" }),
    ).toBeDefined();
  });

  it("見つからないフォルダでは空の格子ではなく案内と最上位への導線を出す", async () => {
    renderFolders("/folders/3/missing");
    expect(await screen.findByText("このフォルダは見つかりません")).toBeDefined();
    const back = screen.getByRole("link", { name: "フォルダの一覧へ" });
    expect(back.getAttribute("href")).toBe("/folders");
    const headings = screen
      .getAllByRole("heading", { level: 2 })
      .map((h) => h.textContent);
    expect(headings).toEqual(["このフォルダは見つかりません"]);
  });

  it("解釈できない URL も見つからない扱いにする", async () => {
    renderFolders("/folders/abc");
    expect(await screen.findByText("このフォルダは見つかりません")).toBeDefined();
  });

  it("再生画面から戻ると控えから復元し、子フォルダも動画も読み直さない", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A");
    await user.click(await screen.findByRole("link", { name: "x" }));
    await user.click(await screen.findByRole("button", { name: "戻る" }));

    expect(await screen.findByRole("link", { name: "x" })).toBeDefined();
    expect(
      screen.getByRole("link", { name: "B、動画 1 本、フォルダ 1 件" }),
    ).toBeDefined();
    expect(requests.filter((url) => url === "/api/folders/3?path=A").length).toBe(1);
    expect(
      requests.filter((url) => url.startsWith("/api/folders/3/videos?")).length,
    ).toBe(1);
  });

  it("別のフォルダの控えでは復元しない", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A");
    // A の控えを取ってから B へ移る。B は A の控えを使わず自分で読む。
    await user.click(
      await screen.findByRole("link", { name: "B、動画 1 本、フォルダ 1 件" }),
    );
    await screen.findByText("このフォルダは見つかりません");
    expect(requests.filter((url) => url === "/api/folders/3?path=A%2FB").length).toBe(1);
  });

  it("取り込み後の読み直しが終わる前に動画を開いても、古い子フォルダを控えに残さない", async () => {
    const user = userEvent.setup();
    let listingCalls = 0;
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) {
        requests.push(url);
        return Promise.resolve(
          json({
            id: 9,
            state: "running",
            total: 1,
            completed: 0,
            failed: 0,
          }),
        );
      }
      if (url === "/api/folders/3?path=A") {
        listingCalls += 1;
        // 取り込み後の子フォルダの読み直し（2回目）だけ、返らないままにする。
        if (listingCalls === 2) {
          requests.push(url);
          return new Promise<Response>(() => {});
        }
      }
      return base!(input, init);
    });
    renderFolders("/folders/3/A");
    await screen.findByRole("link", { name: "x" });
    await emitServerEvent("scan", {
      id: 9,
      state: "done",
      total: 1,
      completed: 1,
      failed: 0,
    });
    await waitFor(() => expect(listingCalls).toBe(2));

    // 動画は読み直し済みで、子フォルダはまだ古い。
    await user.click(await screen.findByRole("link", { name: "x" }));
    await user.click(await screen.findByRole("button", { name: "戻る" }));

    // 控えから復元せず、子フォルダを読み直す。
    await waitFor(() => expect(listingCalls).toBe(3));
  }, 10_000);

  it("最上位でも取り込みが終わると登録フォルダを読み直す", async () => {
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      if (String(input).startsWith("/api/scans/current")) {
        requests.push(String(input));
        return Promise.resolve(
          json({
            id: 9,
            state: "running",
            total: 1,
            completed: 0,
            failed: 0,
          }),
        );
      }
      return base!(input, init);
    });
    renderFolders("/folders");
    await screen.findByRole("link", {
      name: "movies、動画 0 本、フォルダ 9 件、/a/movies",
    });
    expect(requests.filter((url) => url === "/api/folders").length).toBe(1);

    await emitServerEvent("scan", {
      id: 9,
      state: "done",
      total: 1,
      completed: 1,
      failed: 0,
    });
    await waitFor(() =>
      expect(requests.filter((url) => url === "/api/folders").length).toBe(2),
    );
  }, 10_000);

  it("取り込みが終わると子フォルダと動画を読み直す", async () => {
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      if (String(input).startsWith("/api/scans/current")) {
        requests.push(String(input));
        return Promise.resolve(
          json({
            id: 9,
            state: "running",
            total: 1,
            completed: 0,
            failed: 0,
          }),
        );
      }
      return base!(input, init);
    });
    renderFolders("/folders/3/A");
    await screen.findByRole("link", { name: "x" });
    expect(requests.filter((url) => url === "/api/folders/3?path=A").length).toBe(1);

    await emitServerEvent("scan", {
      id: 9,
      state: "done",
      total: 1,
      completed: 1,
      failed: 0,
    });
    await waitFor(() =>
      expect(requests.filter((url) => url === "/api/folders/3?path=A").length).toBe(2),
    );
    await waitFor(() =>
      expect(
        requests.filter((url) => url.startsWith("/api/folders/3/videos?")).length,
      ).toBe(2),
    );
  }, 10_000);

  it("URL の q・watch をそのまま送り、q があるときだけ scope=subtree にする", async () => {
    renderFolders("/folders/3/A?q=京都&watch=unwatched");
    await waitFor(() =>
      expect(
        requests.some(
          (url) =>
            url.startsWith("/api/folders/3/videos?path=A&scope=subtree") &&
            url.includes("query=%E4%BA%AC%E9%83%BD") &&
            url.includes("watch=unwatched"),
        ),
      ).toBe(true),
    );

    requests.length = 0;
    renderFolders("/folders/3/A?watch=unwatched");
    await waitFor(() =>
      expect(
        requests.some(
          (url) =>
            url.startsWith("/api/folders/3/videos?path=A&watch=unwatched") &&
            !url.includes("scope=") &&
            !url.includes("query="),
        ),
      ).toBe(true),
    );
  });

  it("URL の tag は読まず、watch を変えると tag が URL から消える（issue 269）", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A?tag=1");
    await screen.findByRole("link", { name: "x" });
    // tag は読まないので、絞り込みなしと同じ要求になる（tag は送らない）。
    expect(
      requests.some(
        (url) => url.startsWith("/api/folders/3/videos?path=A") && url.includes("tag="),
      ),
    ).toBe(false);

    await user.click(screen.getByRole("button", { name: "絞り込み" }));
    await user.click(screen.getByRole("radio", { name: "未視聴" }));
    await waitFor(() => {
      const location = screen.getByTestId("location").textContent ?? "";
      expect(location).toContain("watch=unwatched");
      expect(location).not.toContain("tag=");
    });
  });

  it("検索語があると子フォルダを出さず、配下の検索結果に置き場所を添える。消すと元に戻る", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A?q=京都");

    const x = await screen.findByRole("link", { name: "x、このフォルダ" });
    const y = await screen.findByRole("link", { name: "y、B" });
    expect(x).toBeDefined();
    expect(y).toBeDefined();
    // 検索中は子フォルダのカードと「フォルダ」「動画」の見出しを出さない。
    expect(screen.queryByRole("link", { name: /^B、/ })).toBeNull();
    expect(screen.getByRole("heading", { level: 2, name: "検索結果" })).toBeDefined();
    const status = screen.getByRole("status");
    expect(status.textContent).toBe("2件");
    expect(status.classList.contains("sr-only")).toBe(false);

    const box = screen.getByRole("searchbox", { name: "Aの中を検索" });
    await user.clear(box);
    await waitFor(() => expect(screen.getByRole("link", { name: "x" })).toBeDefined());
    expect(
      screen.getByRole("link", { name: "B、動画 1 本、フォルダ 1 件" }),
    ).toBeDefined();
  });

  it("検索中にフォルダが無くなると、一致なしではなく見つからない案内を出す", async () => {
    renderFolders("/folders/3/A?q=gone");
    expect(await screen.findByText("このフォルダは見つかりません")).toBeDefined();
    expect(screen.queryByText("条件に一致する動画はありません")).toBeNull();
  });

  it("404 の後に条件を変えて取得に失敗したら、見つからない案内ではなく再試行を出す", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A?q=gone");
    await screen.findByText("このフォルダは見つかりません");
    const box = screen.getByRole("searchbox");
    await user.clear(box);
    await user.type(box, "broken{Enter}");
    expect(await screen.findByText("一覧を取得できません")).toBeDefined();
    expect(screen.queryByText("このフォルダは見つかりません")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("フォルダ検索の再試行中は読み込み表示へ戻す", async () => {
    const base = fetchMock.getMockImplementation();
    let attempts = 0;
    let resolveRetry: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url.startsWith("/api/folders/3/videos?")) {
        const params = new URL(url, "http://localhost").searchParams;
        if (params.get("query") === "broken") {
          attempts++;
          if (attempts === 1) {
            return Promise.resolve(
              json({ code: "internal", message: "壊れています" }, 500),
            );
          }
          return new Promise<Response>((resolve) => {
            resolveRetry = resolve;
          });
        }
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    renderFolders("/folders/3/A?q=broken");
    expect(await screen.findByText("一覧を取得できません")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "再試行" }));
    await waitFor(() => expect(resolveRetry).toBeDefined());
    expect(screen.getByRole("status").textContent).toBe("読み込み中…");
    expect(screen.queryByText("一覧を取得できません")).toBeNull();

    await act(async () => {
      resolveRetry?.(
        json({
          items: [video(1, "x", { folder: { rootId: 3, path: "A" } })],
          total: 1,
        } satisfies VideoPage),
      );
    });
    expect(await screen.findByRole("link", { name: "x、このフォルダ" })).toBeDefined();
  });

  it("最上位検索の再試行中は読み込み表示へ戻す", async () => {
    const base = fetchMock.getMockImplementation();
    let attempts = 0;
    let resolveRetry: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/folders") {
        attempts++;
        if (attempts === 1) {
          return Promise.resolve(
            json({ code: "internal", message: "壊れています" }, 500),
          );
        }
        return new Promise<Response>((resolve) => {
          resolveRetry = resolve;
        });
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    renderFolders("/folders?q=京都");
    expect(await screen.findByText("一覧を取得できません")).toBeDefined();
    expect(screen.queryByRole("link", { name: /^x/ })).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();

    await user.click(screen.getByRole("button", { name: "再試行" }));
    await waitFor(() => expect(resolveRetry).toBeDefined());
    expect(screen.getByRole("status").textContent).toBe("読み込み中…");
    expect(screen.queryByText("一覧を取得できません")).toBeNull();

    await act(async () => {
      resolveRetry?.(json(roots));
    });
    expect(await screen.findByRole("link", { name: "x、movies/A" })).toBeDefined();
  });

  it("最上位の検索で動画と登録フォルダ一覧の両方が失敗したら、1度の再試行で両方を取り直す", async () => {
    const base = fetchMock.getMockImplementation();
    let fail = true;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (fail && (url === "/api/folders" || url.startsWith("/api/videos?"))) {
        return Promise.resolve(json({ code: "internal", message: "壊れています" }, 500));
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    renderFolders("/folders?q=京都");
    expect(await screen.findByText("一覧を取得できません")).toBeDefined();
    fail = false;
    await user.click(screen.getByRole("button", { name: "再試行" }));
    expect(await screen.findByRole("link", { name: "x、movies/A" })).toBeDefined();
  });

  it("最上位の検索はライブラリ全体を対象にし、置き場所を登録フォルダ名から作る", async () => {
    renderFolders("/folders?q=京都");
    const x = await screen.findByRole("link", { name: "x、movies/A" });
    const y = await screen.findByRole("link", { name: "y、movies/A/B" });
    const z = await screen.findByRole("link", { name: "z、movies" });
    expect(x).toBeDefined();
    expect(y).toBeDefined();
    expect(z).toBeDefined();
    expect(requests.some((url) => url.startsWith("/api/videos?query="))).toBe(true);
  });

  it("並び順の向きを切り替えられる", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A?sort=titleDesc");
    await screen.findByRole("link", { name: "x" });
    expect(screen.getByRole("button", { name: "並び順: 題名" })).toBeDefined();

    await user.click(screen.getByRole("button", { name: "降順。押すと昇順" }));
    await waitFor(() =>
      expect(requests.some((url) => url.includes("sort=titleAsc"))).toBe(true),
    );
  });

  it("絞り込みだけでは子フォルダのカードが残り、Nは絞った後のtotalになる", async () => {
    renderFolders("/folders/3/A?watch=unwatched");
    const b = await screen.findByRole("link", {
      name: "B、動画 1 本、フォルダ 1 件",
    });
    expect(b).toBeDefined();
    expect(screen.getByRole("heading", { level: 2, name: /^動画/ }).textContent).toBe(
      "動画 1",
    );
    expect(screen.getByRole("link", { name: "x" })).toBeDefined();
  });

  it("絞り込みだけで一致が無いと、子フォルダの下に簡潔な一致なしを出す", async () => {
    renderFolders("/folders/3/A?watch=watched");
    await screen.findByRole("link", { name: "B、動画 1 本、フォルダ 1 件" });
    expect(
      screen.getByRole("heading", { name: "条件に一致する動画はありません" }),
    ).toBeDefined();
    expect(screen.queryByText(/中のフォルダも探すには/)).toBeNull();
    expect(screen.queryByText("視聴済み")).toBeNull();
    expect(screen.queryByText("Aの直下")).toBeNull();
    expect(screen.queryByRole("button", { name: "条件を解除" })).toBeNull();
  });

  it("検索の一致なしでは検索語や範囲や解除操作を重ねない", async () => {
    renderFolders("/folders/3/A?q=zzz-no-such-video");
    await screen.findByRole("heading", { name: "条件に一致する動画はありません" });
    expect(screen.queryByText("検索語「zzz-no-such-video」")).toBeNull();
    expect(screen.queryByText("Aとその中")).toBeNull();
    expect(screen.queryByRole("button", { name: "条件を解除" })).toBeNull();
    expect(screen.queryByRole("link", { name: /^B、/ })).toBeNull();
  });

  it("sort=random と seed を URL のまま使う", async () => {
    renderFolders("/folders/3/A?sort=random&seed=42");
    await screen.findByRole("link", { name: "x" });
    expect(screen.getByRole("button", { name: "並べ直す" })).toBeDefined();
    await waitFor(() =>
      expect(
        requests.some((url) => url.includes("sort=random") && url.includes("seed=42")),
      ).toBe(true),
    );
  });

  it("並べ直すは新しい seed で履歴を1つ増やし、戻ると前の seed に戻る", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A?sort=random&seed=7");
    await screen.findByRole("link", { name: "x" });

    await user.click(screen.getByRole("button", { name: "並べ直す" }));
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).not.toBe(
        "/folders/3/A?sort=random&seed=7",
      ),
    );
    const seed = new URLSearchParams(
      screen.getByTestId("location").textContent?.split("?")[1] ?? "",
    ).get("seed");
    expect(seed).not.toBe("7");
    await waitFor(() =>
      expect(requests.some((url) => url.includes(`seed=${seed ?? ""}`))).toBe(true),
    );

    await user.click(screen.getByRole("button", { name: "テストで戻る" }));
    expect(screen.getByTestId("location").textContent).toBe(
      "/folders/3/A?sort=random&seed=7",
    );
  });

  it("/ でフォルダの検索欄にフォーカスする", async () => {
    const user = userEvent.setup();
    renderFolders("/folders/3/A");
    await screen.findByRole("link", { name: "x" });
    const box = screen.getByRole("searchbox", { name: "Aの中を検索" });
    expect(document.activeElement).not.toBe(box);

    await user.keyboard("/");
    expect(document.activeElement).toBe(box);
  });

  it("最上位で検索語が空のときは、表示と並び順のまとめの中でも並べ替え・向きを無効にする", async () => {
    const user = userEvent.setup();
    renderFolders("/folders");
    await screen.findAllByRole("link", { name: /^movies、/ });

    await user.click(screen.getByRole("button", { name: "表示と並び順" }));
    // jsdom は <fieldset disabled> から子孫の入力への継承を実装しないので、
    // fieldset 自身が disabled を持つことを確かめる（実ブラウザでは子の
    // input・button にも及ぶ）。絞り込み・並べ替えのメニューボタンは disabled
    // 属性を自分で持つので、そちらは直接確かめられる。
    const group = await screen.findByRole("group", { name: "並び順" });
    expect((group as HTMLFieldSetElement).disabled).toBe(true);
    expect(
      (screen.getByRole("button", { name: "絞り込み" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "並び順: 追加日" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});
