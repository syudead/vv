import type { Video } from "../api/client";

export type PlaybackRoute = "direct" | "transcode";
export type PlaybackState = "loading" | "ready" | "playing" | "failed" | "disposed";

export interface PlaybackAttempt {
  videoId: number;
  durationMs: number;
  route: PlaybackRoute;
  state: PlaybackState;
  fallbackTried: boolean;
  logicalPositionMs: number;
  sourceOffsetMs: number;
  playIntended: boolean;
}

export function createPlaybackAttempt(
  video: Video,
  resumePositionMs: number,
): PlaybackAttempt | null {
  if (
    video.probeState !== "done" ||
    video.durationMs === undefined ||
    video.durationMs <= 0 ||
    video.videoCodec === undefined
  ) {
    return null;
  }
  const position = clampPosition(resumePositionMs, video.durationMs);
  const route: PlaybackRoute = video.playable ? "direct" : "transcode";
  return {
    videoId: video.id,
    durationMs: video.durationMs,
    route,
    state: "loading",
    fallbackTried: false,
    logicalPositionMs: position,
    sourceOffsetMs: route === "transcode" ? position : 0,
    playIntended: false,
  };
}

export function updatePosition(
  attempt: PlaybackAttempt,
  positionMs: number,
): PlaybackAttempt {
  return {
    ...attempt,
    logicalPositionMs: clampPosition(positionMs, attempt.durationMs),
  };
}

export function fallbackToTranscode(
  attempt: PlaybackAttempt,
  positionMs: number,
  playIntended: boolean,
): PlaybackAttempt | null {
  if (attempt.route !== "direct" || attempt.fallbackTried) return null;
  const position = clampTranscodeStart(positionMs, attempt.durationMs);
  return {
    ...attempt,
    route: "transcode",
    state: "loading",
    fallbackTried: true,
    logicalPositionMs: position,
    sourceOffsetMs: position,
    playIntended,
  };
}

export function clampPosition(positionMs: number, durationMs: number): number {
  if (!Number.isFinite(positionMs)) return 0;
  return Math.min(durationMs, Math.max(0, Math.round(positionMs)));
}

export function clampTranscodeStart(positionMs: number, durationMs: number): number {
  // Container durations can extend slightly past the final decodable frame.
  // Leave enough media at the tail for FFmpeg to produce a playable fragment.
  return Math.min(Math.max(0, durationMs - 1000), clampPosition(positionMs, durationMs));
}
