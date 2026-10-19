import type { Video } from "../api/client";

const minResumeMs = 5000;

/** Return the originating list URL or /; reject external URLs. */
export function backTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !from.startsWith("/") || from.startsWith("//")) {
    return "/";
  }
  return from;
}

export function autoplayRequested(state: unknown): boolean {
  return (state as { autoplay?: unknown } | null)?.autoplay === true;
}

/** The `t` query's start in milliseconds, or null without a usable one; `t` is in seconds. */
export function startFromSearch(search: string): number | null {
  const value = new URLSearchParams(search).get("t");
  if (value === null || !/^\d+(\.\d+)?$/.test(value)) return null;
  return Math.round(Number(value) * 1000);
}

/** Versions share progress; restart if the saved position exceeds this version's duration. */
export function resumePosition(video: Video): number {
  const progress = video.progress;
  if (progress === undefined || progress.completed || progress.positionMs < minResumeMs) {
    return 0;
  }
  if (video.durationMs !== undefined && progress.positionMs >= video.durationMs) return 0;
  return progress.positionMs;
}

const maxTrail = 100;

/** Return valid video IDs oldest first; only navigation between video pages adds history. */
export function videoTrail(state: unknown): number[] {
  const trail: unknown = (state as { trail?: unknown } | null)?.trail;
  if (!Array.isArray(trail)) return [];
  return trail.filter(
    (id): id is number => typeof id === "number" && Number.isSafeInteger(id) && id > 0,
  );
}

export interface VideoLinkState {
  /** Preserve the original return destination. */
  from: string;
  trail: number[];
  autoplay?: boolean;
}

export function linkStateFrom(
  backTo: string,
  trail: readonly number[],
  currentId: number,
): VideoLinkState {
  return { from: backTo, trail: [...trail, currentId].slice(-maxTrail) };
}
