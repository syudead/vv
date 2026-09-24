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
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video, VideoPage } from "../api/client";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
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

function renderLibrary(initial = "/") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <ScanProvider>
            <Routes>
              <Route path="/" element={<LibraryPage />} />
              <Route path="/videos/:id" element={<p>再生画面</p>} />
            </Routes>
          </ScanProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

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

  it("一覧と件数を出す", async () => {
    renderLibrary();
    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
    expect(screen.getByRole("status").textContent).toBe("3 件（6:00 · 6.0 MB）");
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("25");
  });

  it("検索語は URL から読んで件数行に出す", async () => {
    renderLibrary("/?q=abc");
    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toMatch(/^「abc」/);
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

  it("絞り込み時は後続ページも取得して全件から探す", async () => {
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
                items: [video(1)],
                total: 2,
                nextCursor: "cursor-1",
              }
            : {
                items: [
                  video(2, {
                    progress: {
                      positionMs: 30_000,
                      completed: false,
                      updatedAt: "",
                    },
                  }),
                ],
                total: 2,
              },
        ),
      );
    });
    const user = userEvent.setup();
    renderLibrary();
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "絞り込み" }));
    await user.click(screen.getByRole("radio", { name: "視聴途中" }));

    expect(await screen.findByRole("link", { name: "動画 2" })).toBeDefined();
    expect(listCalls).toBe(2);
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
