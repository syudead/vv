import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import {
  createPlaybackAttempt,
  fallbackToTranscode,
  updatePosition,
} from "./playbackAttempt";

const video: Video = {
  id: 7,
  title: "test",
  public: false,
  sizeBytes: 100,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  previewState: "pending",
  thumbnailState: "done",
  durationMs: 10_000,
  videoCodec: "h264",
  tags: [],
};

describe("PlaybackAttempt", () => {
  it("解析済み動画だけに初期経路を作る", () => {
    expect(createPlaybackAttempt(video, 2000)).toMatchObject({
      route: "direct",
      logicalPositionMs: 2000,
      sourceOffsetMs: 0,
    });
    expect(createPlaybackAttempt({ ...video, playable: false }, 2000)).toMatchObject({
      route: "transcode",
      sourceOffsetMs: 2000,
    });
    expect(createPlaybackAttempt({ ...video, probeState: "failed" }, 0)).toBeNull();
    expect(createPlaybackAttempt({ ...video, durationMs: undefined }, 0)).toBeNull();
  });

  it("direct errorからtranscodeへ移るのは1回だけ", () => {
    const direct = createPlaybackAttempt(video, 0);
    if (direct === null) throw new Error("attemptがありません");
    const fallback = fallbackToTranscode(direct, 4500, true);
    expect(fallback).toMatchObject({
      route: "transcode",
      fallbackTried: true,
      logicalPositionMs: 4500,
      sourceOffsetMs: 4500,
      playIntended: true,
    });
    expect(
      fallback === null ? null : fallbackToTranscode(fallback, 5000, true),
    ).toBeNull();
  });

  it("論理位置を動画尺へ収める", () => {
    const attempt = createPlaybackAttempt(video, 0);
    if (attempt === null) throw new Error("attemptがありません");
    expect(updatePosition(attempt, 12_000).logicalPositionMs).toBe(10_000);
    expect(fallbackToTranscode(attempt, 10_000, false)?.sourceOffsetMs).toBe(9000);
  });
});
