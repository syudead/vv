import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Link, MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import VideoPage from "./VideoPage";

interface PlayerCallbacks {
  onPosition: (positionMs: number) => void;
  onProgress: (positionMs: number, immediate: boolean) => void;
  onError: () => void;
}

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerCallbacks | undefined,
}));

vi.mock("./VideoPlayer", () => ({
  default: (props: PlayerCallbacks) => {
    playerMock.props = props;
    return <div data-testid="video-player" />;
  },
  canStartPlayback: () => true,
}));

const video: Video = {
  id: 7,
  title: "テスト動画",
  sizeBytes: 84_331_821,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "pending",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPage(id = "7", from?: string) {
  return render(
    <MemoryRouter
      initialEntries={[
        { pathname: `/videos/${id}`, state: from === undefined ? undefined : { from } },
      ]}
    >
      <Link to="/videos/8">次の動画</Link>
      <Link to="/videos/invalid">無効な動画</Link>
      <Routes>
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("VideoPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    playerMock.props = undefined;
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockResolvedValue(json(video));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("題名・解像度・詳細タブを出す", async () => {
    renderPage();
    expect(
      await screen.findByRole("heading", { level: 1, name: "テスト動画" }),
    ).toBeDefined();
    expect(screen.getAllByText("1920×1080")).toHaveLength(2);
    expect(screen.getByText("4:02")).toBeDefined();
    expect(screen.getByRole("tab", { name: "ファイル情報" })).toBeDefined();
    expect(document.title).toBe("テスト動画 - vv");
  });

  it("戻り先は遷移元の一覧、無ければ /", async () => {
    renderPage("7", "/?q=abc&sort=titleAsc");
    const back = await screen.findByRole("link", { name: "ライブラリ" });
    expect(back.getAttribute("href")).toBe("/?q=abc&sort=titleAsc");
  });

  it("フォルダ画面から来たら、戻り先をフォルダと名付ける", async () => {
    renderPage("7", "/folders/3/A%20B?sort=titleAsc");
    const back = await screen.findByRole("link", { name: "フォルダ" });
    expect(back.getAttribute("href")).toBe("/folders/3/A%20B?sort=titleAsc");
  });

  it("外部 URL を戻り先にしない", async () => {
    renderPage("7", "//evil.example");
    const back = await screen.findByRole("link", { name: "ライブラリ" });
    expect(back.getAttribute("href")).toBe("/");
  });

  it("解析済みの非対応動画はライブ変換から開始する", async () => {
    fetchMock.mockResolvedValue(
      json({
        ...video,
        playable: false,
        unplayableReason: "video_codec",
        videoCodec: "hevc",
      }),
    );
    renderPage();
    await screen.findByRole("heading", { level: 1, name: "テスト動画" });
    expect(screen.getByTestId("video-player")).toBeDefined();
    expect(screen.queryByText("この動画は再生できません")).toBeNull();
  });

  it("取得に失敗したら理由を出す", async () => {
    fetchMock.mockResolvedValue(
      json({ code: "not_found", message: "見つかりません" }, 404),
    );
    renderPage();
    expect(await screen.findByText("見つかりません")).toBeDefined();
  });

  it("playerの即時保存通知をprogress APIへ送る", async () => {
    renderPage();
    await screen.findByRole("heading", { level: 1, name: "テスト動画" });
    const callbacks = playerMock.props;
    if (callbacks === undefined) throw new Error("player が描画されていません");

    callbacks.onProgress(12_345, true);
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            String(input) === "/api/videos/7/progress" &&
            init?.method === "PUT" &&
            init.body === JSON.stringify({ positionMs: 12_345 }),
        ),
      ).toBe(true);
    });

    callbacks.onProgress(242_000, true);
    await waitFor(() => {
      const progressCalls = fetchMock.mock.calls.filter(
        ([input, init]) =>
          String(input) === "/api/videos/7/progress" && init?.method === "PUT",
      );
      expect(progressCalls).toHaveLength(2);
      expect(progressCalls[1]?.[1]?.body).toBe(JSON.stringify({ positionMs: 242_000 }));
    });
  });

  it("DOM ref が外れた後のアンマウントでも最後の再生位置を送る", async () => {
    const page = renderPage();
    await screen.findByRole("heading", { level: 1, name: "テスト動画" });
    const callbacks = playerMock.props;
    if (callbacks === undefined) throw new Error("player が描画されていません");

    callbacks.onPosition(12_345);
    page.unmount();

    const finalCall = fetchMock.mock.calls
      .filter(
        ([input, init]) =>
          String(input) === "/api/videos/7/progress" && init?.keepalive === true,
      )
      .at(-1);
    expect(finalCall?.[1]).toEqual(
      expect.objectContaining({
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ positionMs: 12_345 }),
        keepalive: true,
      }),
    );
  });

  it("別動画へのroute変更で前の動画の進捗を新しいIDへ送らない", async () => {
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/videos/7") return Promise.resolve(json(video));
      if (String(input) === "/api/videos/8") return new Promise(() => undefined);
      return Promise.resolve(json({}));
    });
    const page = renderPage();
    await screen.findByRole("heading", { level: 1, name: "テスト動画" });
    const callbacks = playerMock.props;
    if (callbacks === undefined) throw new Error("player が描画されていません");
    callbacks.onPosition(12_345);

    fireEvent.click(screen.getByRole("link", { name: "次の動画" }));
    await waitFor(() => expect(screen.queryByTestId("video-player")).toBeNull());
    page.unmount();

    const finalProgressURLs = fetchMock.mock.calls
      .filter(([, init]) => init?.keepalive === true)
      .map(([input]) => String(input));
    expect(finalProgressURLs).toEqual(["/api/videos/7/progress"]);
  });

  it("無効なrouteへ変更したら前の動画の再生エラーを消す", async () => {
    renderPage();
    await screen.findByRole("heading", { level: 1, name: "テスト動画" });
    const callbacks = playerMock.props;
    if (callbacks === undefined) throw new Error("player が描画されていません");
    callbacks.onError();
    expect(await screen.findByRole("alert")).toBeDefined();

    fireEvent.click(screen.getByRole("link", { name: "無効な動画" }));

    expect(await screen.findByText("動画の指定が正しくありません")).toBeDefined();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
