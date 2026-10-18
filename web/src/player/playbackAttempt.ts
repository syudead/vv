import type { Video } from "../api/client";
import type { PlaybackQuality } from "../preferences/playbackQuality";

export type PlaybackRoute = "direct" | "transcode";
export type PlaybackState = "loading" | "ready" | "playing" | "failed" | "disposed";

export interface PlaybackAttempt {
  videoId: number;
  durationMs: number;
  route: PlaybackRoute;
  /** Non-original qualities always use live transcoding. */
  quality: PlaybackQuality;
  state: PlaybackState;
  fallbackTried: boolean;
  logicalPositionMs: number;
  sourceOffsetMs: number;
  playIntended: boolean;
}

/** Quality must already be validated by effectiveQuality; only original quality can use direct playback. */
export function createPlaybackAttempt(
  video: Video,
  resumePositionMs: number,
  quality: PlaybackQuality = "original",
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
  const route: PlaybackRoute =
    quality === "original" && video.playable ? "direct" : "transcode";
  return {
    videoId: video.id,
    durationMs: video.durationMs,
    route,
    quality,
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

/** Preserve transcode fallback when returning to original quality. Direct sources start at zero, so callers seek after metadata. */
export function switchQuality(
  attempt: PlaybackAttempt,
  quality: PlaybackQuality,
  playable: boolean,
  positionMs: number,
  playIntended: boolean,
): PlaybackAttempt {
  const route: PlaybackRoute =
    quality === "original" && playable && !attempt.fallbackTried ? "direct" : "transcode";
  const position =
    route === "transcode"
      ? clampTranscodeStart(positionMs, attempt.durationMs)
      : clampPosition(positionMs, attempt.durationMs);
  return {
    ...attempt,
    route,
    quality,
    state: "loading",
    logicalPositionMs: position,
    sourceOffsetMs: route === "transcode" ? position : 0,
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
