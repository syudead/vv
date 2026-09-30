import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import {
  createPlaybackAttempt,
  fallbackToTranscode,
  switchQuality,
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

  it("元の画質以外は直接再生できる動画でも位置から変換で始め、元の画質は今までどおり", () => {
    expect(createPlaybackAttempt(video, 2000, "480p")).toMatchObject({
      route: "transcode",
      quality: "480p",
      logicalPositionMs: 2000,
      sourceOffsetMs: 2000,
    });
    expect(
      createPlaybackAttempt({ ...video, playable: false }, 2000, "480p"),
    ).toMatchObject({ route: "transcode", quality: "480p", sourceOffsetMs: 2000 });
    expect(createPlaybackAttempt(video, 2000, "original")).toMatchObject({
      route: "direct",
      quality: "original",
      sourceOffsetMs: 0,
    });
    expect(createPlaybackAttempt(video, 2000)).toMatchObject({ quality: "original" });
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

describe("switchQuality", () => {
  it("縮めた画質は切り替えた位置からの変換にし、再生の意図を保つ", () => {
    const attempt = createPlaybackAttempt(video, 0);
    if (attempt === null) throw new Error("attempt");
    expect(switchQuality(attempt, "480p", true, 4000, true)).toMatchObject({
      route: "transcode",
      quality: "480p",
      state: "loading",
      logicalPositionMs: 4000,
      sourceOffsetMs: 4000,
      playIntended: true,
    });
  });

  it("元の画質は直接再生できる動画なら直接再生に戻し、位置は論理上の位置に持つ", () => {
    const attempt = createPlaybackAttempt(video, 0, "480p");
    if (attempt === null) throw new Error("attempt");
    expect(switchQuality(attempt, "original", true, 9800, false)).toMatchObject({
      route: "direct",
      quality: "original",
      logicalPositionMs: 9800,
      sourceOffsetMs: 0,
      playIntended: false,
    });
    expect(switchQuality(attempt, "original", false, 4000, false)).toMatchObject({
      route: "transcode",
      sourceOffsetMs: 4000,
    });
  });

  it("直接再生が読めず変換へ切り替えた動画は、元の画質でも変換のままにする", () => {
    const attempt = createPlaybackAttempt(video, 0);
    if (attempt === null) throw new Error("attempt");
    const fallback = fallbackToTranscode(attempt, 3000, true);
    if (fallback === null) throw new Error("fallback");
    const lowered = switchQuality(fallback, "360p", true, 3000, true);
    expect(switchQuality(lowered, "original", true, 3000, true)).toMatchObject({
      route: "transcode",
      quality: "original",
    });
  });
});
