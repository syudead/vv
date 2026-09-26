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

import {
  emitServerEvent,
  FakeEventSource,
  installFakeEventSource,
} from "../api/fakeEventSource";

import type {
  FolderListing,
  FolderSummary,
  RootFolderListing,
  Video,
  VideoPage,
} from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import FolderCard from "./FolderCard";
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
    public: false,
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

function renderFolders(initial: string, audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
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
          </AudienceProvider>
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
        items: [
          video(1, "x", {
            tags: [{ id: 5, name: "旅行", manual: true, fromFolder: false }],
          }),
        ],
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

  it("フォルダの絵柄の上でマウスを横に動かすと、位置に応じたサムネイルを前に出す", async () => {
    const { container } = renderFolders("/folders/3");
    await screen.findByRole("link", { name: "five、動画 6 本、フォルダ 0 件" });
    const art = container.querySelector<HTMLElement>(
      '[data-folder-path="five"] [data-folder-art]',
    );
    if (art === null) throw new Error("folder art not found");
    art.getBoundingClientRect = () => new DOMRect(100, 0, 200, 100);
    const front = () => art.querySelector("[data-folder-front]");

    fireEvent.pointerMove(art, { pointerType: "touch", clientX: 290 });
    expect(front()).toBeNull();

    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 110 });
    expect(art.querySelectorAll("[data-folder-front]").length).toBe(1);
    expect(front()?.querySelector("img")?.getAttribute("src")).toBe(
      "/api/videos/1/thumbnail?v=x",
    );

    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 290 });
    expect(front()?.querySelector("img")?.getAttribute("src")).toBe(
      "/api/videos/4/thumbnail?v=x",
    );

    fireEvent.pointerLeave(art);
    expect(front()).toBeNull();
  });

  it("1件だけのフォルダでも、マウスを乗せるとサムネイルを前に出す", () => {
    const one = summary({
      path: "one",
      name: "one",
      videoCount: 1,
      previews: previews(1),
    });
    const { container } = render(
      <MemoryRouter>
        <FolderCard folder={one} showPath={false} />
      </MemoryRouter>,
    );
    const art = container.querySelector<HTMLElement>("[data-folder-art]");
    if (art === null) throw new Error("folder art not found");
    art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 150 });
    expect(art.querySelector("[data-folder-front]")).not.toBeNull();
  });

  it("前に出た1枚は、少し待ってからプレビュー動画を流し、外れると止める", async () => {
    vi.useFakeTimers();
    const play = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockResolvedValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
    const load = vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
    try {
      const folder = summary({
        path: "two",
        name: "two",
        videoCount: 2,
        previews: [
          {
            videoId: 1,
            thumbnailUrl: "/api/videos/1/thumbnail?v=x",
            previewUrl: "/api/videos/1/preview?v=x",
          },
          { videoId: 2, thumbnailUrl: "/api/videos/2/thumbnail?v=x" },
        ],
      });
      const { container } = render(
        <MemoryRouter>
          <FolderCard folder={folder} showPath={false} />
        </MemoryRouter>,
      );
      const art = container.querySelector<HTMLElement>("[data-folder-art]");
      if (art === null) throw new Error("folder art not found");
      art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
      const videos = () =>
        Array.from(art.querySelectorAll("video"), (video) => video.getAttribute("src"));

      fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
      act(() => vi.advanceTimersByTime(300));
      expect(videos()).toEqual([]);
      act(() => vi.advanceTimersByTime(150));
      expect(videos()).toEqual(["/api/videos/1/preview?v=x"]);
      expect(art.querySelector("[data-folder-front] video")).not.toBeNull();
      expect(play).toHaveBeenCalled();

      // プレビュー動画の無い1枚に移ると、前には出すが動画は流さない。
      fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 190 });
      act(() => vi.advanceTimersByTime(500));
      expect(videos()).toEqual([]);
      expect(load).toHaveBeenCalled();

      fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
      act(() => vi.advanceTimersByTime(500));
      expect(videos()).toHaveLength(1);
      fireEvent.pointerLeave(art);
      expect(videos()).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("再生を断られたら、読み込みを解いてサムネイルのまま前に出しておく", async () => {
    vi.useFakeTimers();
    let played: HTMLMediaElement | null = null;
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function (
      this: HTMLMediaElement,
    ) {
      played = this;
      return Promise.reject(new Error("denied"));
    });
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
    const load = vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
    try {
      const folder = summary({
        path: "one",
        name: "one",
        videoCount: 1,
        previews: [
          {
            videoId: 1,
            thumbnailUrl: "/api/videos/1/thumbnail?v=x",
            previewUrl: "/api/videos/1/preview?v=x",
          },
        ],
      });
      const { container } = render(
        <MemoryRouter>
          <FolderCard folder={folder} showPath={false} />
        </MemoryRouter>,
      );
      const art = container.querySelector<HTMLElement>("[data-folder-art]");
      if (art === null) throw new Error("folder art not found");
      art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
      fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
      await act(async () => {
        vi.advanceTimersByTime(450);
        await Promise.resolve();
      });
      const video = played as HTMLMediaElement | null;
      if (video === null) throw new Error("play was not called");
      expect(art.querySelector("video")).toBeNull();
      expect(video.hasAttribute("src")).toBe(false);
      expect(load).toHaveBeenCalled();
      expect(art.querySelector("[data-folder-front] img")).not.toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("プレビュー動画が読めなければ、サムネイルのまま前に出しておく", () => {
    vi.useFakeTimers();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockRejectedValue(new Error("nope"));
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
    vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
    try {
      const folder = summary({
        path: "one",
        name: "one",
        videoCount: 1,
        previews: [
          {
            videoId: 1,
            thumbnailUrl: "/api/videos/1/thumbnail?v=x",
            previewUrl: "/api/videos/1/preview?v=x",
          },
        ],
      });
      const { container } = render(
        <MemoryRouter>
          <FolderCard folder={folder} showPath={false} />
        </MemoryRouter>,
      );
      const art = container.querySelector<HTMLElement>("[data-folder-art]");
      if (art === null) throw new Error("folder art not found");
      art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
      fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
      act(() => vi.advanceTimersByTime(450));
      const video = art.querySelector("video");
      if (video === null) throw new Error("video not mounted");
      fireEvent.error(video);
      expect(art.querySelector("video")).toBeNull();
      expect(art.querySelector("[data-folder-front] img")).not.toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("下見の位置がプレビューの件数を超えたら、下見をやめる", () => {
    const four = summary({
      path: "four",
      name: "four",
      videoCount: 4,
      previews: previews(4),
    });
    const { container, rerender } = render(
      <MemoryRouter>
        <FolderCard folder={four} showPath={false} />
      </MemoryRouter>,
    );
    const art = container.querySelector<HTMLElement>("[data-folder-art]");
    if (art === null) throw new Error("folder art not found");
    art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 190 });
    expect(art.querySelector("[data-folder-front]")).not.toBeNull();

    rerender(
      <MemoryRouter>
        <FolderCard folder={{ ...four, previews: previews(2) }} showPath={false} />
      </MemoryRouter>,
    );
    expect(art.querySelector("[data-folder-front]")).toBeNull();
    expect(art.querySelectorAll("[data-folder-preview]").length).toBe(2);
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

  // 受け入れ条件 18: グループはライブラリでだけ1件にまとまる。フォルダ画面の一覧と
  // 最上位の検索結果は、今のまま動画を1本ずつ出す（GET /api/library を使わない。
  // specs/017-folder-groups/plan.md の Structural Decisions 6）。
  it("フォルダの一覧と最上位の検索結果は、グループにまとめず1本ずつ出す", async () => {
    renderFolders("/folders?q=京都");
    expect(await screen.findByRole("link", { name: "x、movies/A" })).toBeDefined();
    expect(screen.getByRole("link", { name: "y、movies/A/B" })).toBeDefined();
    cleanup();

    renderFolders("/folders/3/A");
    expect(await screen.findByRole("link", { name: "x" })).toBeDefined();
    expect(requests.some((url) => url.startsWith("/api/folders/3/videos?"))).toBe(true);
    expect(requests.some((url) => url.startsWith("/api/library"))).toBe(false);
    expect(document.querySelector("[data-group-root]")).toBeNull();
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
  describe("ゲスト（specs/016-single-account-auth/ui-design.md「Guest degradation」）", () => {
    /** withoutRootPath はゲストの応答（登録フォルダの絶対パスが無い）を作る。 */
    function withoutRootPath({ rootPath: _, ...rest }: FolderSummary): FolderSummary {
      return rest;
    }

    function guestResponses(extra?: (url: string) => Response | undefined) {
      fetchMock.mockImplementation((input) => {
        const url = String(input);
        requests.push(url);
        const custom = extra?.(url);
        if (custom !== undefined) return Promise.resolve(custom);
        if (url === "/api/folders") {
          return Promise.resolve(json({ folders: roots.folders.map(withoutRootPath) }));
        }
        if (url === "/api/folders/3?path=A") {
          return Promise.resolve(
            json({
              folder: withoutRootPath(folderA.folder),
              folders: folderA.folders.map(withoutRootPath),
            }),
          );
        }
        if (url === "/api/folders/3") {
          return Promise.resolve(
            json({
              folder: withoutRootPath(folderRoot.folder),
              folders: folderRoot.folders.map(withoutRootPath),
            }),
          );
        }
        if (url.startsWith("/api/folders/3/videos?")) {
          return Promise.resolve(json({ items: [video(1, "x")], total: 1 }));
        }
        return Promise.resolve(json({ code: "unauthorized", message: "x" }, 401));
      });
    }

    it("最上位の登録フォルダにパスを添えず、所有者だけの API も /api/events も開かない", async () => {
      guestResponses();
      const { container } = renderFolders("/folders", "guest");
      await screen.findByRole("link", { name: "movies、動画 0 本、フォルダ 9 件" });
      expect(
        screen.getByRole("link", { name: "movies、動画 1 本、フォルダ 0 件" }),
      ).toBeDefined();
      expect(container.textContent).not.toContain("/a/movies");
      expect(requests).toEqual(["/api/folders"]);
      expect(FakeEventSource.instances).toHaveLength(0);
    });

    it("公開の動画が無ければ、設定でなくログインへの入口を出す", async () => {
      guestResponses((url) =>
        url === "/api/folders" ? json({ folders: [] }) : undefined,
      );
      renderFolders("/folders", "guest");
      expect(await screen.findByText("公開されている動画はありません")).toBeDefined();
      expect(screen.queryByRole("link", { name: "設定を開く" })).toBeNull();
      expect(screen.getByRole("link", { name: "ログイン" }).getAttribute("href")).toBe(
        "/login?next=%2Ffolders",
      );
    });

    it("子フォルダのパンくずの登録フォルダの段を、登録フォルダの name から作る", async () => {
      guestResponses();
      renderFolders("/folders/3/A", "guest");
      const nav = await screen.findByRole("navigation", { name: "パンくず" });
      const root = await within(nav).findByRole("link", { name: "movies" });
      expect(root.getAttribute("href")).toBe("/folders/3");
      expect(root.getAttribute("title")).toBe("movies");
      expect(requests).toContain("/api/folders/3");
    });

    it("登録フォルダの name を読めなければ、その段を骨組みのまま残さずに省く", async () => {
      guestResponses((url) =>
        url === "/api/folders/3"
          ? json({ code: "not_found", message: "見つかりません" }, 404)
          : undefined,
      );
      renderFolders("/folders/3/A", "guest");
      const nav = await screen.findByRole("navigation", { name: "パンくず" });
      await waitFor(() => expect(requests).toContain("/api/folders/3"));
      await waitFor(() =>
        expect(
          within(nav)
            .getAllByRole("link")
            .map((link) => link.textContent),
        ).toEqual(["フォルダ"]),
      );
      expect(within(nav).getByText("A").getAttribute("aria-current")).toBe("page");
    });

    it("URL に残った watch と最近再生した順は既定に丸めて要求し、URL も直す", async () => {
      guestResponses();
      renderFolders("/folders/3/A?watch=watched&sort=playedAsc", "guest");
      await screen.findByRole("link", { name: "x" });
      const videoRequests = requests
        .filter((url) => url.startsWith("/api/folders/3/videos?"))
        .map((url) => new URL(url, "http://localhost").searchParams);
      expect(videoRequests.length).toBeGreaterThan(0);
      for (const params of videoRequests) {
        expect(params.get("watch") ?? "all").toBe("all");
        expect(params.get("sort")).toBe("addedDesc");
      }
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).toBe(
          "/folders/3/A?sort=addedDesc",
        ),
      );
    });
  });
  describe("まとめ方のメニュー（specs/017-folder-groups/ui-design.md「Folder grouping menu」）", () => {
    const series: FolderListing = {
      folder: summary({
        path: "series",
        name: "series",
        videoCount: 2,
        grouping: { mode: "auto", grouped: true, taggable: true },
      }),
      folders: [],
    };
    const rootWithVideos: FolderListing = {
      folder: summary({
        videoCount: 1,
        grouping: { mode: "auto", grouped: false, taggable: false },
      }),
      folders: [],
    };
    const writes: { method: string; url: string; body: unknown }[] = [];

    /** respond は、まとめ方の経路の応答を差し替えた偽のサーバーを入れる。 */
    function respond(grouping: (method: string, url: string) => Response) {
      writes.length = 0;
      fetchMock.mockImplementation((input, init) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        requests.push(url);
        if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
        if (url.includes("/grouping")) {
          writes.push({
            method,
            url,
            body: typeof init?.body === "string" ? JSON.parse(init.body) : undefined,
          });
          return Promise.resolve(grouping(method, url));
        }
        if (url === "/api/folders/3?path=series") return Promise.resolve(json(series));
        if (url === "/api/folders/3") return Promise.resolve(json(rootWithVideos));
        if (url.startsWith("/api/folders/3/videos?")) {
          return Promise.resolve(
            json({ items: [video(1, "ep01"), video(2, "ep02")], total: 2 }),
          );
        }
        if (url === "/api/tags") return Promise.resolve(json([]));
        return Promise.resolve(json({ code: "not_found", message: "x" }, 404));
      });
    }

    /** holdLibrarySnapshot はライブラリの一覧の控えを置く（再生画面から戻る前の状態）。 */
    function holdLibrarySnapshot() {
      saveListSnapshot(
        { query: "" },
        {
          items: [{ kind: "video", video: video(1, "ep01") }],
          total: 1,
          hasMore: false,
          scrollY: 0,
        },
      );
      expect(takeListSnapshot({ query: "" })).toBeDefined();
    }

    function trigger(grouped: boolean) {
      return screen.findByRole("button", {
        name: `ライブラリでのまとめ方: ${grouped ? "1 件にまとめて表示" : "1 本ずつ表示"}。メニューを開く`,
      });
    }

    it("「動画 N」の行に今のまとめ方を出し、ラジオで PUT を送って応答で文言を変え、ライブラリの控えを捨てる（受け入れ条件 3・4・5）", async () => {
      respond(() => json({ mode: "ungroup", grouped: false, taggable: false }));
      const user = userEvent.setup();
      renderFolders("/folders/3/series");
      const button = await trigger(true);
      expect(button.textContent).toBe("ライブラリで 1 件");
      expect(button.closest("section")?.querySelector("h2")?.textContent).toBe("動画 2");
      const listingReads = requests.filter(
        (url) => url === "/api/folders/3?path=series",
      ).length;
      const videoReads = requests.filter((url) =>
        url.startsWith("/api/folders/3/videos?"),
      ).length;
      holdLibrarySnapshot();

      await user.click(button);
      const menu = await screen.findByRole("menu");
      expect(within(menu).getByText("ライブラリでのまとめ方")).toBeDefined();
      expect(
        within(menu)
          .getAllByRole("menuitemradio")
          .map((item) => [item.textContent, item.getAttribute("aria-checked")]),
      ).toEqual([
        ["自動", "true"],
        ["まとめを解除", "false"],
        ["直下をまとめる", "false"],
      ]);
      expect(
        within(menu).getByRole("menuitem", { name: "グループをタグに変える" }),
      ).toBeDefined();
      await user.click(within(menu).getByRole("menuitemradio", { name: "まとめを解除" }));

      expect((await trigger(false)).textContent).toBe("ライブラリで 1 本ずつ");
      expect(writes).toEqual([
        {
          method: "PUT",
          url: "/api/folders/3/grouping?path=series",
          body: { mode: "ungroup" },
        },
      ]);
      // 次にライブラリを開くと、控えではなく読み直した一覧が出る。
      expect(takeListSnapshot({ query: "" })).toBeUndefined();
      // フォルダ画面の一覧は1本ずつのままなので読み直さない。
      expect(requests.filter((url) => url === "/api/folders/3?path=series").length).toBe(
        listingReads,
      );
      expect(
        requests.filter((url) => url.startsWith("/api/folders/3/videos?")).length,
      ).toBe(videoReads);
      // タグに変えられないフォルダでは項目を出さない。
      await user.click(await trigger(false));
      expect(
        within(await screen.findByRole("menu")).queryByRole("menuitem", {
          name: "グループをタグに変える",
        }),
      ).toBeNull();
    });

    it("同じ値を選び直しても送る", async () => {
      respond(() => json({ mode: "auto", grouped: true, taggable: true }));
      const user = userEvent.setup();
      renderFolders("/folders/3/series");
      await user.click(await trigger(true));
      await user.click(await screen.findByRole("menuitemradio", { name: "自動" }));
      await waitFor(() =>
        expect(writes).toEqual([
          {
            method: "PUT",
            url: "/api/folders/3/grouping?path=series",
            body: { mode: "auto" },
          },
        ]),
      );
    });

    it("グループをタグに変えるとトーストで伝え、動画の一覧を読み直し、ライブラリの控えを捨てる", async () => {
      respond(() =>
        json({
          tag: { id: 5, name: "series" },
          created: true,
          grouping: { mode: "ungroup", grouped: false, taggable: false },
        }),
      );
      const user = userEvent.setup();
      renderFolders("/folders/3/series");
      await user.click(await trigger(true));
      const videoReads = requests.filter((url) =>
        url.startsWith("/api/folders/3/videos?"),
      ).length;
      holdLibrarySnapshot();
      await user.click(
        await screen.findByRole("menuitem", { name: "グループをタグに変える" }),
      );

      expect(
        await screen.findByText("タグ「series」を作り、まとめを解除しました"),
      ).toBeDefined();
      expect(writes.map(({ method, url }) => [method, url])).toEqual([
        ["POST", "/api/folders/3/grouping/tag?path=series"],
      ]);
      expect(await trigger(false)).toBeDefined();
      await waitFor(() =>
        expect(
          requests.filter((url) => url.startsWith("/api/folders/3/videos?")).length,
        ).toBe(videoReads + 1),
      );
      expect(takeListSnapshot({ query: "" })).toBeUndefined();
    });

    it("タグ化の後、元の位置が続きのページにあれば、そこに届くまで読んでスクロールの位置を戻す", async () => {
      const pageHeight = 3000;
      let loadedPages = 0;
      respond(() =>
        json({
          tag: { id: 5, name: "series" },
          created: true,
          grouping: { mode: "ungroup", grouped: false, taggable: false },
        }),
      );
      const base = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        const url = String(input);
        if (!url.startsWith("/api/folders/3/videos?")) return base!(input, init);
        requests.push(url);
        const cursor = new URL(url, "http://localhost").searchParams.get("cursor");
        const index = cursor === null ? 0 : Number(cursor);
        loadedPages = index + 1;
        const page: VideoPage = {
          items: [
            video(index * 2 + 1, `ep${String(index)}a`),
            video(index * 2 + 2, `ep${String(index)}b`),
          ],
          total: 8,
          ...(index < 3 ? { nextCursor: String(index + 1) } : {}),
        };
        return Promise.resolve(json(page));
      });
      const scrollTo = vi.fn();
      vi.stubGlobal("scrollTo", scrollTo);
      Object.defineProperty(document.documentElement, "scrollHeight", {
        configurable: true,
        get: () => loadedPages * pageHeight,
      });
      Object.defineProperty(window, "scrollY", { configurable: true, value: 7000 });
      try {
        const user = userEvent.setup();
        renderFolders("/folders/3/series");
        await user.click(await trigger(true));
        await user.click(
          await screen.findByRole("menuitem", { name: "グループをタグに変える" }),
        );
        // 7000px に届くのは 3 ページ目を読んだ後（3 × 3000 − 768）。4 ページ目は読まない。
        await waitFor(() => expect(loadedPages).toBe(3));
        await waitFor(() =>
          expect(scrollTo).toHaveBeenLastCalledWith({ top: 7000, behavior: "auto" }),
        );
        const calls = scrollTo.mock.calls.length;
        await new Promise((resolve) => setTimeout(resolve, 50));
        expect(loadedPages).toBe(3);
        expect(scrollTo.mock.calls.length).toBe(calls);
      } finally {
        Reflect.deleteProperty(document.documentElement, "scrollHeight");
        Reflect.deleteProperty(window, "scrollY");
      }
    });

    it("タグ化の失敗をトーストで伝え、引き金を戻す", async () => {
      respond(() =>
        json({ code: "invalid_request", message: "タグの名前に使えません" }, 400),
      );
      const user = userEvent.setup();
      renderFolders("/folders/3/series");
      await user.click(await trigger(true));
      await user.click(
        await screen.findByRole("menuitem", { name: "グループをタグに変える" }),
      );
      expect(
        await screen.findByText(
          "「series」はタグの名前に使えないため、タグに変えられません",
        ),
      ).toBeDefined();
      const button = await trigger(true);
      await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    });

    it("409 では「もうグループではありません」と伝え、フォルダの一覧を取り直す", async () => {
      respond(() => json({ code: "conflict", message: "x" }, 409));
      const user = userEvent.setup();
      renderFolders("/folders/3/series");
      await user.click(await trigger(true));
      const listingReads = requests.filter(
        (url) => url === "/api/folders/3?path=series",
      ).length;
      holdLibrarySnapshot();
      await user.click(
        await screen.findByRole("menuitem", { name: "グループをタグに変える" }),
      );
      expect(
        await screen.findByText("このフォルダはもうグループではありません"),
      ).toBeDefined();
      // ほかのタブで先に変わったので、ライブラリの控えも古い。次に開くときは読み直させる。
      expect(takeListSnapshot({ query: "" })).toBeUndefined();
      await waitFor(() =>
        expect(
          requests.filter((url) => url === "/api/folders/3?path=series").length,
        ).toBe(listingReads + 1),
      );
    });

    it("404 は「このフォルダは見つかりません」、ほかは「変更できませんでした」", async () => {
      let status = 404;
      respond(() => json({ code: "x", message: "x" }, status));
      const user = userEvent.setup();
      renderFolders("/folders/3/series");
      await user.click(await trigger(true));
      await user.click(
        await screen.findByRole("menuitemradio", { name: "まとめを解除" }),
      );
      expect(await screen.findByText("このフォルダは見つかりません")).toBeDefined();
      status = 500;
      await user.click(await trigger(true));
      await user.click(
        await screen.findByRole("menuitemradio", { name: "まとめを解除" }),
      );
      expect(await screen.findByText("変更できませんでした")).toBeDefined();
    });

    it("登録フォルダそのものの画面にも出し、送るときは path を省く", async () => {
      respond(() => json({ mode: "groupDirect", grouped: true, taggable: false }));
      const user = userEvent.setup();
      renderFolders("/folders/3");
      await user.click(await trigger(false));
      await user.click(
        await screen.findByRole("menuitemradio", { name: "直下をまとめる" }),
      );
      expect(await trigger(true)).toBeDefined();
      expect(writes.map(({ url }) => url)).toEqual(["/api/folders/3/grouping"]);
    });

    it("検索結果と最上位には出さない", async () => {
      respond(() => json({}));
      renderFolders("/folders/3/series?q=ep");
      await screen.findByRole("status");
      await waitFor(() =>
        expect(requests.some((url) => url.includes("query=ep"))).toBe(true),
      );
      expect(screen.queryByRole("button", { name: /ライブラリでのまとめ方/ })).toBeNull();
    });

    it("ゲストの応答には grouping が無いので、引き金を出さない", async () => {
      fetchMock.mockImplementation((input) => {
        const url = String(input);
        requests.push(url);
        if (url === "/api/folders/3?path=series") {
          const { grouping: _, rootPath: __, ...folder } = series.folder;
          return Promise.resolve(json({ folder, folders: [] }));
        }
        if (url === "/api/folders/3") {
          const { grouping: _, rootPath: __, ...folder } = rootWithVideos.folder;
          return Promise.resolve(json({ folder, folders: [] }));
        }
        if (url.startsWith("/api/folders/3/videos?")) {
          return Promise.resolve(json({ items: [video(1, "ep01")], total: 1 }));
        }
        return Promise.resolve(json({ code: "unauthorized", message: "x" }, 401));
      });
      renderFolders("/folders/3/series", "guest");
      await screen.findByRole("link", { name: /ep01/ });
      expect(
        screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
      ).toContain("動画 1");
      expect(screen.queryByRole("button", { name: /まとめ方/ })).toBeNull();
    });
  });
});
