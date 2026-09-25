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

import type { Video, VideoPage } from "../api/client";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import { resultCountText } from "../videoList/listSummary";
import LibraryPage from "./LibraryPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024 * 1024 * id,
    addedAt: "2026-09-01T00:00:00Z",
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
    .filter((url) => url.pathname === "/api/videos");
}

function renderLibrary(initial = "/") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
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
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("resultCountText", () => {
  it("サーバーの全件数だけを表示する", () => {
    expect(resultCountText(59)).toBe("59件");
    expect(resultCountText(1_234)).toBe("1,234件");
  });
});

describe("LibraryPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    __resetTagsForTest();
    clearListSnapshot();
    vi.stubGlobal("fetch", fetchMock);
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
        return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
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
    expect(status.textContent).toBe("3件");
    expect(status.classList.contains("sr-only")).toBe(false);
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("25");
  });

  it("検索語は URL から読み、結果件数だけを出す", async () => {
    renderLibrary("/?q=abc");
    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toBe("3件");
    });
    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => String(call[0]));
      expect(calls.some((url) => url.includes("query=abc"))).toBe(true);
    });
  });

  it("狭い画面向けメニューから省略された表示操作を使える", async () => {
    const user = userEvent.setup();
    renderLibrary();

    await user.click(screen.getByRole("button", { name: "表示と並び順" }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("radio", { name: "題名" })).toBeDefined();
    expect(
      within(dialog).getByRole("radiogroup", { name: "表示形式（コンパクト）" }),
    ).toBeDefined();
    expect(within(dialog).getByRole("slider", { name: "カードの大きさ" })).toBeDefined();
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
    expect(await screen.findByText("動画がまだありません")).toBeDefined();
    expect(screen.getByRole("button", { name: "取り込む" })).toBeDefined();
  });

  it("失敗なら再試行を出し、再試行中は読み込み表示へ戻す", async () => {
    let attempts = 0;
    let resolveRetry: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
      }
      attempts++;
      if (attempts === 1) {
        return Promise.resolve(json({ code: "internal", message: "壊れています" }, 500));
      }
      return new Promise<Response>((resolve) => {
        resolveRetry = resolve;
      });
    });
    const user = userEvent.setup();
    renderLibrary();
    expect(await screen.findByText("壊れています")).toBeDefined();
    expect(screen.getByRole("button", { name: "再試行" })).toBeDefined();
    expect(screen.queryByRole("status")).toBeNull();

    await user.click(screen.getByRole("button", { name: "再試行" }));
    await waitFor(() => expect(resolveRetry).toBeDefined());
    expect(screen.getByRole("status").textContent).toBe("読み込み中…");
    expect(screen.queryByText("壊れています")).toBeNull();

    await act(async () => {
      resolveRetry?.(json({ items: [video(1)], total: 1 } satisfies VideoPage));
    });
    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
  });

  it("条件変更後の取得に失敗したら前の件数を残さない", async () => {
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/videos" && url.searchParams.get("query") === "broken") {
        return Promise.resolve(json({ code: "internal", message: "壊れています" }, 500));
      }
      return base!(input, init);
    });
    const user = userEvent.setup();
    renderLibrary();
    expect((await screen.findByRole("status")).textContent).toBe("3件");

    await user.type(screen.getByRole("searchbox", { name: "動画を検索" }), "broken");

    expect(await screen.findByText("壊れています")).toBeDefined();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("視聴状態はサーバーに送り、残りのページを読みに行かず、件数はサーバーの total を出す", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
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

    await user.click(screen.getByRole("button", { name: "絞り込み" }));
    await user.click(screen.getByRole("radio", { name: "未視聴" }));

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "動画 2" })).toBeNull(),
    );
    expect(screen.getByRole("status").textContent).toBe("250件");
    const requests = listRequests(fetchMock);
    expect(requests.at(-1)?.searchParams.get("watch")).toBe("unwatched");
    // 選んだ瞬間に続きのページを読みに行かない。
    expect(requests.filter((url) => url.searchParams.has("cursor"))).toHaveLength(0);
    expect(screen.getByTestId("location").textContent).toBe(
      "?watch=unwatched&sort=addedDesc",
    );
    expect(screen.getByRole("button", { name: "絞り込み（1 件適用中）" })).toBeDefined();
  });

  it("URL の条件をそのまま一覧の要求に載せる", async () => {
    renderLibrary("/?q=%E4%BA%AC%E9%83%BD&watch=inProgress&playable=1&sort=durationAsc");
    await screen.findByRole("link", { name: "動画 1" });
    const request = listRequests(fetchMock).at(-1);
    expect(request?.searchParams.get("query")).toBe("京都");
    expect(request?.searchParams.get("watch")).toBe("inProgress");
    expect(request?.searchParams.get("playable")).toBe("true");
    expect(request?.searchParams.get("sort")).toBe("durationAsc");
    expect(screen.getByRole("button", { name: "並び順: 長さ" })).toBeDefined();
    expect(
      screen.getByRole("button", { name: "昇順（短い順）。押すと降順" }),
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
    expect(screen.getByRole("button", { name: "並べ直す" })).toBeDefined();
  });

  it("並べ直すは新しい seed で読み直し、戻るで前の並びに戻る", async () => {
    const user = userEvent.setup();
    renderLibrary("/?sort=random&seed=7");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getAllByRole("button", { name: "並べ直す" })[0]!);
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
      screen.getByRole("button", { name: "降順（新しい順）。押すと昇順" }),
    );
    expect(screen.getByTestId("location").textContent).toBe("?sort=addedAsc");
    await waitFor(() =>
      expect(listRequests(fetchMock).at(-1)?.searchParams.get("sort")).toBe("addedAsc"),
    );
    expect(
      screen.getByRole("button", { name: "昇順（古い順）。押すと降順" }),
    ).toBeDefined();

    // 最初の URL（sort なし）も戻り先として並び順を持つので、保存した並び順が
    // 変わっても戻ると前の並びになる。
    await user.click(screen.getByRole("button", { name: "テストで戻る" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=addedDesc");
    expect(
      screen.getByRole("button", { name: "降順（新しい順）。押すと昇順" }),
    ).toBeDefined();
  });

  it("メニューで種類を選ぶと、その種類の選んだときの向きになる", async () => {
    const user = userEvent.setup();
    renderLibrary("/?sort=addedAsc");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "並び順: 追加日" }));
    const items = await screen.findAllByRole("menuitemradio");
    expect(items.map((item) => item.textContent)).toEqual([
      "追加日",
      "更新日時",
      "題名",
      "長さ",
      "ファイルサイズ",
      "最近再生した順",
      "ランダム",
    ]);
    await user.click(screen.getByRole("menuitemradio", { name: "ファイルサイズ" }));
    expect(screen.getByTestId("location").textContent).toBe("?sort=sizeDesc");
  });

  it("検索語の入力は一続きで履歴を1つだけ増やす", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });

    const box = screen.getByRole("searchbox", { name: "動画を検索" });
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

    expect(await screen.findByText("条件に一致する動画はありません")).toBeDefined();
    expect(screen.queryByText("検索語「京都」")).toBeNull();
    expect(screen.queryByText("未視聴")).toBeNull();
    expect(screen.queryByText("再生できるものだけ")).toBeNull();
    expect(screen.queryByRole("button", { name: "条件を解除" })).toBeNull();
    expect(screen.queryByText("動画がまだありません")).toBeNull();
  });

  it("絞り込みの条件を解除は検索語も外し、ポップオーバーを閉じて絞り込みのボタンへ戻る", async () => {
    const user = userEvent.setup();
    renderLibrary("/?q=abc&sort=titleAsc");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "絞り込み" }));
    await user.click(await screen.findByRole("button", { name: "条件を解除" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTestId("location").textContent).toBe("?sort=titleAsc");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "絞り込み" }),
      ),
    );
  });

  it("キーボードだけで検索欄 → × → 手引き → 絞り込み → 並べ替え → 向きの順に進み、手引きは Tab で閉じる", async () => {
    const user = userEvent.setup();
    renderLibrary("/?q=abc&sort=addedDesc");
    await screen.findByRole("link", { name: "動画 1" });

    const box = screen.getByRole("searchbox", { name: "動画を検索" });
    const help = screen.getByRole("button", { name: "検索の書き方" });
    box.focus();
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "検索語をクリア" }),
    );
    await user.tab();
    expect(document.activeElement).toBe(help);

    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getAllByRole("term")).toHaveLength(4);
    expect(document.activeElement).toBe(help);

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "絞り込み" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect((box as HTMLInputElement).value).toBe("abc");
    expect(screen.getByTestId("location").textContent).toBe("?q=abc&sort=addedDesc");

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "並び順: 追加日" }),
    );
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "降順（新しい順）。押すと昇順" }),
    );
  });

  it("手引きを開いて閉じても検索語と URL は変わらない", async () => {
    const user = userEvent.setup();
    renderLibrary("/?q=abc&sort=addedDesc");
    await screen.findByRole("link", { name: "動画 1" });

    const help = screen.getByRole("button", { name: "検索の書き方" });
    await user.click(help);
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    expect(document.activeElement).toBe(help);
    expect(
      (screen.getByRole("searchbox", { name: "動画を検索" }) as HTMLInputElement).value,
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

  it("選択すると選択バーが出て Esc で消える", async () => {
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });
    const checkbox = screen.getByRole("checkbox", { name: "「動画 1」を選択" });
    expect(checkbox.parentElement?.className).toContain(
      "[@media(hover:none)]:opacity-100",
    );
    await user.click(checkbox);
    expect(screen.getByText("1 件を選択中")).toBeDefined();
    await user.keyboard("{Escape}");
    expect(screen.queryByText("1 件を選択中")).toBeNull();
  });

  it("preview は一度に1件だけ active にし resize で全 card を reset する", async () => {
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
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
        return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
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
    fireEvent.click(screen.getByRole("button", { name: "絞り込み" }));
    fireEvent.click(screen.getByRole("radio", { name: "未視聴" }));
    await act(async () => Promise.resolve());
    expect(document.querySelector("video")).toBeNull();

    startFirst();
    fireEvent.click(screen.getByRole("button", { name: "表示と並び順" }));
    let dialog = screen.getByRole("dialog");
    const slider = within(dialog).getByRole("slider", { name: "カードの大きさ" });
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(document.querySelector("video")).toBeNull();

    startFirst();
    fireEvent.click(within(dialog).getByRole("radio", { name: "リスト" }));
    expect(document.querySelector("video")).toBeNull();
    expect(screen.queryByRole("article")).toBeNull();

    dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("radio", { name: "グリッド" }));
    startFirst();
    dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("radio", { name: "題名" }));
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
        return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
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
          return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
        }
        if (url.pathname === "/api/tags") return Promise.resolve(tagsResponse(tags));
        if (url.pathname === "/api/videos") {
          const requestedTag = url.searchParams.getAll("tag").map(Number);
          return Promise.resolve(json(makeItems(requestedTag) satisfies VideoPage));
        }
        throw new Error(`unexpected request: ${url.toString()}`);
      });
    }

    it("/?tag=1&sort=random は seed を補い、tag=1 を残す", async () => {
      const tag = { id: 1, name: "旅行" };
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
      const tag = { id: 1, name: "旅行" };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?tag=1");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      expect(screen.getByText("1 件を選択中")).toBeDefined();

      // 一覧の上のチップでタグの絞り込みを外すと、同じ動画がまだ見えていても
      // 選択は解除する（検索語・視聴状態・再生可否と同じ扱い）。
      await user.click(screen.getByRole("button", { name: "旅行の絞り込みを外す" }));
      await waitFor(() => expect(screen.queryByText("1 件を選択中")).toBeNull());
    });

    it("カードのタグを押すと絞り込みに加わり、上の行に出る。すでに絞り込み中のタグは変わらない", async () => {
      const tag = { id: 1, name: "旅行" };
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

      await user.click(screen.getByRole("button", { name: "旅行で絞り込む" }));
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).toContain("tag=1"),
      );
      expect(
        within(screen.getByRole("list", { name: "絞り込み中のタグ" })).getByText("旅行"),
      ).toBeDefined();

      // すでに絞り込み中のタグをカードでもう一度押しても何も変わらない。
      const before = screen.getByTestId("location").textContent;
      await user.click(screen.getByRole("button", { name: "旅行で絞り込む" }));
      expect(screen.getByTestId("location").textContent).toBe(before);
    });

    it("タグを押しても、検索語・視聴状態などのほかの条件は残る（N1）", async () => {
      const tag = { id: 1, name: "旅行" };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?q=abc&watch=unwatched");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "旅行で絞り込む" }));

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
      const extra = { id: 17, name: "17個目" };
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

      await user.click(screen.getByRole("button", { name: "17個目で絞り込む" }));
      expect(await screen.findByText("絞り込めるタグは 16 個までです")).toBeDefined();
      expect(screen.getByTestId("location").textContent).not.toContain("tag=17");
    });

    it("絞り込み中のタグを外すと、その id だけが消えてほかの条件は残る", async () => {
      const tagA = { id: 1, name: "旅行" };
      const tagB = { id: 2, name: "2024" };
      installTagAwareList([tagA, tagB], () => ({
        items: [video(1, { tags: [tagA, tagB] })],
        total: 1,
      }));
      const user = userEvent.setup();
      renderLibrary("/?tag=1&tag=2&q=abc");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("button", { name: "2024の絞り込みを外す" }));
      await waitFor(() => {
        const location = screen.getByTestId("location").textContent ?? "";
        expect(location).toContain("tag=1");
        expect(location).not.toContain("tag=2&");
        expect(location).not.toMatch(/tag=2$/);
        expect(location).toContain("q=abc");
      });
    });

    it("missingTagIds を受けたら伝えて、タグの一覧を取り直し、一覧も取り直して URL から取り除く（N1）", async () => {
      const tagA = { id: 1, name: "旅行" };
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
        await screen.findByText("削除されたタグを絞り込みから外しました"),
      ).toBeDefined();

      // タグの一覧（/api/tags）を取り直す。
      await waitFor(() => {
        const tagRequests = fetchMock.mock.calls.filter((call) =>
          new URL(String(call[0]), "http://localhost").pathname.startsWith("/api/tags"),
        );
        expect(tagRequests.length).toBeGreaterThanOrEqual(2);
      });
      // 一覧（/api/videos）も、tag を外した条件で取り直す。
      await waitFor(() => {
        const videoRequests = fetchMock.mock.calls.filter((call) => {
          const url = new URL(String(call[0]), "http://localhost");
          return url.pathname === "/api/videos" && !url.searchParams.has("tag");
        });
        expect(videoRequests.length).toBeGreaterThanOrEqual(1);
      });
    });

    it("控えから戻したときも、共有のタグの一覧と突き合わせてもう無い id を取り除く。一覧も取り直す（N1）", async () => {
      const { saveListSnapshot } = await import("../api/listSnapshot");
      saveListSnapshot(
        { query: "", sort: "addedDesc", tags: [1] },
        { items: [video(1)], total: 1, hasMore: false, scrollY: 0 },
      );
      // 共有のタグの一覧にはもう id 1 が無い（別のタブで削除された想定）。
      installTagAwareList([], () => ({ items: [video(1)], total: 1 }));
      // sort を明示し、端末に保存された並び順（前のテストの操作の影響）に
      // 依らず控えの鍵を "addedDesc" に固定する。
      renderLibrary("/?tag=1&sort=addedDesc");

      // 控えを使うので、最初の /api/videos は要求しない。
      expect(
        fetchMock.mock.calls.filter(
          (call) =>
            new URL(String(call[0]), "http://localhost").pathname === "/api/videos",
        ),
      ).toHaveLength(0);

      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).not.toContain("tag=1"),
      );
      expect(
        await screen.findByText("削除されたタグを絞り込みから外しました"),
      ).toBeDefined();

      // タグの一覧を取り直し（画面が開くとき + 突き合わせで最低2回）、条件が
      // 変わったので一覧（/api/videos）も取り直す。
      await waitFor(() => {
        const tagRequests = fetchMock.mock.calls.filter((call) =>
          new URL(String(call[0]), "http://localhost").pathname.startsWith("/api/tags"),
        );
        expect(tagRequests.length).toBeGreaterThanOrEqual(1);
      });
      await waitFor(() => {
        const videoRequests = fetchMock.mock.calls.filter(
          (call) =>
            new URL(String(call[0]), "http://localhost").pathname === "/api/videos",
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
          items: [video(99, { title: "控えの動画" })],
          total: 1,
          hasMore: false,
          scrollY: 0,
        },
      );
      const tag = { id: 1, name: "旅行" };
      installTagAwareList([tag], () => ({
        items: [video(1, { tags: [tag] })],
        total: 1,
      }));

      renderLibrary("/?tag=1");

      // 控え（控えの動画）ではなく、要求した一覧（動画 1）が出る。
      expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
      expect(screen.queryByRole("link", { name: "控えの動画" })).toBeNull();
      // 控えを使わないので、通常どおり /api/videos を要求する。
      expect(
        fetchMock.mock.calls.some(
          (call) =>
            new URL(String(call[0]), "http://localhost").pathname === "/api/videos",
        ),
      ).toBe(true);
    });

    it("タグだけで絞って0件のとき、該当なしにタグのチップが出て、条件を解除でタグが外れる", async () => {
      const tag = { id: 1, name: "旅行" };
      installTagAwareList([tag], () => ({ items: [], total: 0 }));
      const user = userEvent.setup();
      renderLibrary("/?tag=1");

      expect(await screen.findByText("条件に一致する動画はありません")).toBeDefined();
      expect(
        within(screen.getByRole("list", { name: "絞り込み中のタグ" })).getByText("旅行"),
      ).toBeDefined();

      await user.click(screen.getByRole("button", { name: "絞り込み" }));
      await user.click(await screen.findByRole("button", { name: "条件を解除" }));
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
      /** true にすると /api/videos/ids の応答を、呼び出し元が明示的に流すまで止める。 */
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
          return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
        }
        if (url.pathname === "/api/tags" && method === "GET") {
          return Promise.resolve(
            json({ items: tags.map((tag) => ({ ...tag, synonyms: [], videoCount: 1 })) }),
          );
        }
        if (url.pathname === "/api/videos/ids") {
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
          }));
          return Promise.resolve(json({ total: body.videoIds.length, items }));
        }
        if (url.pathname === "/api/videos") {
          return Promise.resolve(
            json({
              items: [
                video(1, {
                  tags: [...(attached.get(1) ?? [])].map((id) => ({ id, name: "旅行" })),
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

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      expect(screen.getByText("1 件を選択中")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "すべて選択" }));
      expect(await screen.findByText("50 件を選択中")).toBeDefined();
    });

    it("すべて選択が失敗したら、選択を変えずにトーストで伝える", async () => {
      installSelectionAwareList({ total: 50, idsFail: true });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("button", { name: "すべて選択" }));

      expect(await screen.findByText("すべてを選択できませんでした")).toBeDefined();
      expect(screen.getByText("1 件を選択中")).toBeDefined();
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

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("button", { name: "すべて選択" }));
      expect(await screen.findByRole("button", { name: "選択中…" })).toBeDefined();

      // 応答がまだ届かない間に、手動で選択を変える（この要求はもう当てはまらない）。
      await user.click(screen.getByRole("checkbox", { name: "「動画 2」を選択" }));

      // ボタンは「選択中…」で固まらず、「すべて選択」に戻る。
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "すべて選択" })).toBeDefined(),
      );
      expect(screen.getByText("2 件を選択中")).toBeDefined();

      // 遅れて届いた応答（もう無効）は、手動で選んだ2件を上書きしない。
      await act(async () => {
        resolveIds();
        await Promise.resolve();
      });
      expect(screen.getByText("2 件を選択中")).toBeDefined();
    });

    it("絞り込み中のタグを別のタブで消してから「すべて選択」すると、選ばれず、もう無いことが伝わる", async () => {
      const tag = { id: 1, name: "旅行" };
      installSelectionAwareList({ tags: [], total: 3, idsMissingTagIds: [1] });
      const user = userEvent.setup();
      renderLibrary("/?tag=1");
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("button", { name: "すべて選択" }));

      expect(
        await screen.findByText("削除されたタグを絞り込みから外しました"),
      ).toBeDefined();
      await waitFor(() =>
        expect(screen.getByTestId("location").textContent).not.toContain("tag=1"),
      );
      // 選択は作られない。条件（tag）が変わったことで、押す前の選択も解除される
      // （検索語・視聴状態・再生可否と同じ扱い。ui-design.md「Active tag filters」）。
      await waitFor(() => expect(screen.queryByText(/件を選択中/)).toBeNull());
      void tag;
    });

    it("選択バーでタグを付けると、読み込み済みのカードにすぐ出て、選択は残る", async () => {
      const tag = { id: 1, name: "旅行" };
      installSelectionAwareList({ tags: [tag] });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("checkbox", { name: "「動画 2」を選択" }));
      await user.click(screen.getByRole("button", { name: "タグを付ける" }));
      const input = await screen.findByRole("combobox", { name: "タグを付ける" });
      await user.type(input, "旅行");
      await screen.findByRole("option", { name: /旅行/ });
      await user.keyboard("{Enter}");

      expect(await screen.findByText("2 件に「旅行」を付けました")).toBeDefined();
      // 選択は残る。
      expect(screen.getByText("2 件を選択中")).toBeDefined();
      // 読み込み済みのカード（動画1）にタグがすぐ出る（#267 の通知）。可視の行
      // （タグの一覧）だけを見る。オーバーフロー計測用の隠れた複製とは別に数える。
      const card = screen.getByText("動画 1").closest("article");
      expect(card).not.toBeNull();
      const tagList = within(card as HTMLElement).getByRole("list", { name: "タグ" });
      expect(within(tagList).getByText("旅行")).toBeDefined();
    });

    it("Esc で選択バーのポップオーバーだけを閉じ、選択は残る（キーボード確認 手順2）", async () => {
      installSelectionAwareList({ tags: [{ id: 1, name: "旅行" }] });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("button", { name: "タグを外す" }));
      await screen.findByText("選んだ動画にタグはありません");

      await user.keyboard("{Escape}");
      await waitFor(() =>
        expect(screen.queryByText("選んだ動画にタグはありません")).toBeNull(),
      );
      // ポップオーバーだけが閉じ、選択バー自体（選択）は残る。
      expect(screen.getByText("1 件を選択中")).toBeDefined();
    });

    // B1: 以前は items が変わるたびに、選択を items に無い id ごと刈り込んで
    // いた。タグの付け外しも loadMore も items を新しい配列に置き換えるので、
    // 「すべて選択」でまだ読み込んでいない id まで選んでいると、それらが
    // 巻き込まれて消えていた（Plan の Structural Decisions 4、完了の条件2）。
    it("すべて選択のあとタグを付けても、選択の件数は変わらない（B1）", async () => {
      installSelectionAwareList({
        tags: [{ id: 1, name: "旅行" }],
        total: 50,
        allIds: Array.from({ length: 50 }, (_, i) => i + 1),
      });
      const user = userEvent.setup();
      renderLibrary();
      await screen.findByRole("link", { name: "動画 1" });

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("button", { name: "すべて選択" }));
      expect(await screen.findByText("50 件を選択中")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "タグを付ける" }));
      const input = await screen.findByRole("combobox", { name: "タグを付ける" });
      await user.type(input, "旅行");
      await screen.findByRole("option", { name: /旅行/ });
      await user.keyboard("{Enter}");

      expect(await screen.findByText("50 件に「旅行」を付けました")).toBeDefined();
      // 読み込み済みのカードにタグが反映されて items が新しい配列になっても、
      // 選択の件数（読み込んでいない分を含む）はそのまま。
      expect(screen.getByText("50 件を選択中")).toBeDefined();
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
          return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
        }
        if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
        if (url.pathname === "/api/videos/ids") {
          return Promise.resolve(json({ ids: [1, 2, 3, 4, 5] }));
        }
        if (url.pathname === "/api/videos") {
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

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      await user.click(screen.getByRole("button", { name: "すべて選択" }));
      expect(await screen.findByText("5 件を選択中")).toBeDefined();

      act(() =>
        intersect?.(
          [{ isIntersecting: true } as IntersectionObserverEntry],
          {} as IntersectionObserver,
        ),
      );
      expect(await screen.findByRole("link", { name: "動画 4" })).toBeDefined();
      // loadMore で items が伸びても、選択の件数はそのまま。
      expect(screen.getByText("5 件を選択中")).toBeDefined();
    });

    // Devin の指摘4: 一括で外したタグが今の絞り込みに含まれているときは、選択を
    // 解除して一覧を取り直し、件数と一覧を条件に合わせ直す
    // （docs/design-docs/library-ui.md §6）。
    it("一括で外したタグが今の絞り込みに含まれるとき、選択を解除して一覧を取り直す（Devin の指摘4）", async () => {
      const tag = { id: 1, name: "旅行" };
      const attached = new Map<number, Set<number>>([[1, new Set([1])]]);
      fetchMock.mockImplementation((input, init) => {
        const url = new URL(String(input), "http://localhost");
        const method = init?.method ?? "GET";
        if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
        if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
        if (url.pathname === "/api/processing") {
          return Promise.resolve(json({ probe: 0, thumbnail: 0, preview: 0 }));
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
              items: count > 0 ? [{ tag, count }] : [],
            }),
          );
        }
        if (url.pathname === "/api/videos") {
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

      await user.click(screen.getByRole("checkbox", { name: "「動画 1」を選択" }));
      expect(screen.getByText("1 件を選択中")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "タグを外す" }));
      const removeOption = await screen.findByRole("option", { name: /旅行/ });
      await user.click(removeOption);
      expect(await screen.findByText("1 件から「旅行」を外しました")).toBeDefined();

      // 選択は解除され、絞り込み（tag=1）に合う動画が無くなった一覧に取り直す。
      await waitFor(() => expect(screen.queryByText(/件を選択中/)).toBeNull());
      expect(await screen.findByText("条件に一致する動画はありません")).toBeDefined();
    });
  });
});
