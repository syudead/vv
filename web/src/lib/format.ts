import type { Video } from "../api/client";
import { t, type UiText } from "../i18n";

// Locale-independent media formats; dates, relative times and numbers live in i18n/.

/** Return empty when duration is unknown. */
export function formatDuration(durationMs: number | undefined): string {
  if (durationMs === undefined || durationMs < 0) {
    return "";
  }
  const total = Math.floor(durationMs / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${String(h)}:${pad(m)}:${pad(s)}` : `${String(m)}:${pad(s)}`;
}

/** Use base-1024 units. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return "";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  const digits = index === 0 ? 0 : value >= 100 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[index]}`;
}

/** Return empty if either dimension is missing. */
export function formatResolution(video: Pick<Video, "width" | "height">): string {
  if (video.width === undefined || video.height === undefined) {
    return "";
  }
  return `${String(video.width)}×${String(video.height)}`;
}

export function qualityLabel(video: Pick<Video, "width" | "height">): string {
  const h = video.height;
  const w = video.width;
  if (h === undefined || w === undefined) {
    return "";
  }
  const shorter = Math.min(w, h);
  if (shorter >= 2160) return "4K";
  if (shorter >= 1440) return "1440p";
  if (shorter >= 1080) return "1080p";
  if (shorter >= 720) return "720p";
  if (shorter >= 480) return "480p";
  return `${String(shorter)}p`;
}

export function unplayableText(video: Video): UiText | null {
  // A virtual video has no file, which its mark already says; it is not a file that fails to play.
  if (video.playable || video.virtual) {
    return null;
  }
  if (video.probeState === "failed") {
    return t.video.unplayable.failed;
  }
  if (video.probeState === "pending") {
    return t.video.unplayable.pending;
  }
  if (
    video.durationMs === undefined ||
    video.durationMs <= 0 ||
    video.videoCodec === undefined
  ) {
    return t.video.unplayable.missingInfo;
  }
  return null;
}

export type WatchState = "unwatched" | "inProgress" | "watched";

export function watchState(video: Pick<Video, "progress">): WatchState {
  const p = video.progress;
  if (p === undefined) return "unwatched";
  if (p.completed) return "watched";
  return p.positionMs > 0 ? "inProgress" : "unwatched";
}

/** Return null unless partially watched; otherwise return a ratio in [0, 1]. */
export function watchedRatio(
  video: Pick<Video, "progress" | "durationMs">,
): number | null {
  if (watchState(video) !== "inProgress") return null;
  if (video.durationMs === undefined || video.durationMs <= 0) return null;
  const ratio = (video.progress?.positionMs ?? 0) / video.durationMs;
  return ratio <= 0 ? null : Math.min(ratio, 1);
}

/**
 * Prefer display aspect ratio, falling back to resolution for legacy videos; unknown dimensions
 * are not narrow.
 */
export function isNarrowVideo(
  video: Pick<Video, "width" | "height" | "displayAspectRatio">,
): boolean {
  const ratio =
    video.displayAspectRatio ??
    (video.width !== undefined && video.height !== undefined && video.height > 0
      ? video.width / video.height
      : undefined);
  return ratio !== undefined && ratio > 0 && ratio < 1.25;
}
