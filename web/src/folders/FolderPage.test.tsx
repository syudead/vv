import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

function video(id: number, title: string): Video {
  return {
    id,
    title,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
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

function renderFolders(initial: string) {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <ScanProvider>
            <Routes>
              <Route path="/folders/*" element={<FolderPage />} />
              <Route path="/videos/:id" element={<Player />} />
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
      if (url.startsWith("/api/folders/3/videos?path=A&")) {
        const page: VideoPage = { items: [video(1, "x")], total: 1 };
        return Promise.resolve(json(page));
      }
      if (url.startsWith("/api/folders/3/videos?")) {
        return Promise.resolve(json({ items: [], total: 0 }));
      }
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

  it("最上位でも取り込みが終わると登録フォルダを読み直す", async () => {
    let scanPolls = 0;
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      if (String(input).startsWith("/api/scans/current")) {
        scanPolls += 1;
        requests.push(String(input));
        return Promise.resolve(
          json({
            id: 9,
            state: scanPolls <= 2 ? "running" : "done",
            total: 1,
            completed: scanPolls <= 2 ? 0 : 1,
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

    await waitFor(
      () => expect(requests.filter((url) => url === "/api/folders").length).toBe(2),
      { timeout: 5000 },
    );
  }, 10_000);

  it("取り込みが終わると子フォルダと動画を読み直す", async () => {
    let scanPolls = 0;
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      if (String(input).startsWith("/api/scans/current")) {
        scanPolls += 1;
        requests.push(String(input));
        return Promise.resolve(
          json({
            id: 9,
            state: scanPolls <= 2 ? "running" : "done",
            total: 1,
            completed: scanPolls <= 2 ? 0 : 1,
            failed: 0,
          }),
        );
      }
      return base!(input, init);
    });
    renderFolders("/folders/3/A");
    await screen.findByRole("link", { name: "x" });
    expect(requests.filter((url) => url === "/api/folders/3?path=A").length).toBe(1);

    await waitFor(
      () =>
        expect(requests.filter((url) => url === "/api/folders/3?path=A").length).toBe(2),
      { timeout: 5000 },
    );
    await waitFor(() =>
      expect(
        requests.filter((url) => url.startsWith("/api/folders/3/videos?")).length,
      ).toBe(2),
    );
  }, 10_000);
});
