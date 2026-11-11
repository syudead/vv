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

/**
 * The history entry key of the list that opened the player. The player carries it and passes it
 * back on close, so a list that keeps state in memory per entry, such as the library's image
 * search, restores it on the new entry that closing pushes.
 */
export function returnEntry(state: unknown): string | undefined {
  const entry: unknown = (state as { returnEntry?: unknown } | null)?.returnEntry;
  return typeof entry === "string" && entry !== "" ? entry : undefined;
}

/** The state that keeps the way back: the list URL and, when the list sent one, its entry key. */
export interface ReturnState {
  from: string;
  returnEntry?: string;
}

export function returnState(backTo: string, entry: string | undefined): ReturnState {
  return entry === undefined ? { from: backTo } : { from: backTo, returnEntry: entry };
}

/** Closing goes to the list URL as a new entry, carrying the list's entry key back to it. */
export function closePlayer(back: ReturnState): {
  to: string;
  state: { returnEntry: string } | undefined;
} {
  const { from, returnEntry: entry } = back;
  return { to: from, state: entry === undefined ? undefined : { returnEntry: entry } };
}

export function autoplayRequested(state: unknown): boolean {
  return (state as { autoplay?: unknown } | null)?.autoplay === true;
}

/** The `t` query's start in milliseconds, or null without a usable one; `t` is in seconds. */
export function startFromSearch(search: string): number | null {
  const value = new URLSearchParams(search).get("t");
  if (value === null || !/^\d+(\.\d+)?$/.test(value)) return null;
  // A long digit string overflows to Infinity, which the player rejects.
  const ms = Math.round(Number(value) * 1000);
  return Number.isFinite(ms) ? ms : null;
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

/** Preserve the original return destination. */
export interface VideoLinkState extends ReturnState {
  trail: number[];
  autoplay?: boolean;
}

export function linkStateFrom(
  back: ReturnState,
  trail: readonly number[],
  currentId: number,
): VideoLinkState {
  return { ...back, trail: [...trail, currentId].slice(-maxTrail) };
}
