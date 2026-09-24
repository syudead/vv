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
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import { resultCountText } from "../videoList/listSummary";
import LibraryPage from "./LibraryPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
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

  it("失敗なら再試行を出す", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input).startsWith("/api/scans/current")
          ? json({}, 404)
          : String(input) === "/api/media-folders"
            ? json([{}])
            : json({ code: "internal", message: "壊れています" }, 500),
      ),
    );
    renderLibrary();
    expect(await screen.findByText("壊れています")).toBeDefined();
    expect(screen.getByRole("button", { name: "再試行" })).toBeDefined();
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
});
