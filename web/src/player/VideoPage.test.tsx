import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import VideoPage from "./VideoPage";

const video: Video = {
  id: 7,
  title: "テスト動画",
  sizeBytes: 84_331_821,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
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
      <Routes>
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("VideoPage", () => {
  const fetchMock = vi.fn<typeof fetch>();
  const sendBeaconMock = vi.fn((_url: string, _data?: BodyInit | null) => true);

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("navigator", { sendBeacon: sendBeaconMock });
    fetchMock.mockResolvedValue(json(video));
    sendBeaconMock.mockClear();
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

  it("外部 URL を戻り先にしない", async () => {
    renderPage("7", "//evil.example");
    const back = await screen.findByRole("link", { name: "ライブラリ" });
    expect(back.getAttribute("href")).toBe("/");
  });

  it("再生できない動画は理由を出して video を置かない", async () => {
    fetchMock.mockResolvedValue(
      json({
        ...video,
        playable: false,
        unplayableReason: "video_codec",
        videoCodec: "hevc",
      }),
    );
    renderPage();
    expect(await screen.findByText("この動画は再生できません")).toBeDefined();
    expect(document.querySelector("video")).toBeNull();
  });

  it("取得に失敗したら理由を出す", async () => {
    fetchMock.mockResolvedValue(
      json({ code: "not_found", message: "見つかりません" }, 404),
    );
    renderPage();
    expect(await screen.findByText("見つかりません")).toBeDefined();
  });

  it("一時停止と再生終了で現在位置を直ちに保存する", async () => {
    renderPage();
    await screen.findByRole("heading", { level: 1, name: "テスト動画" });
    const player = document.querySelector("video");
    if (player === null) throw new Error("video が描画されていません");

    player.currentTime = 12.345;
    fireEvent.pause(player);
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

    player.currentTime = 242;
    fireEvent.ended(player);
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
    const player = document.querySelector("video");
    if (player === null) throw new Error("video が描画されていません");

    player.currentTime = 12.345;
    fireEvent.timeUpdate(player);
    page.unmount();

    expect(sendBeaconMock).toHaveBeenCalledOnce();
    const [target, payload] = sendBeaconMock.mock.calls[0] ?? [];
    expect(target).toBe("/api/videos/7/progress");
    expect(payload).toBeInstanceOf(Blob);
    expect((payload as Blob).type).toBe("application/json");
    expect(await (payload as Blob).text()).toBe(JSON.stringify({ positionMs: 12_345 }));
  });
});
