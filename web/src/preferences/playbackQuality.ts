import { isTranscodeQuality, type TranscodeQuality } from "../api/client";

export type PlaybackQuality = "original" | TranscodeQuality;

const storageKey = "vv.playback-quality.v1";

export const defaultPlaybackQuality: PlaybackQuality = "original";

/** Return original quality if storage is absent, invalid or unreadable. */
export function readPlaybackQuality(storage?: Storage): PlaybackQuality {
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(storageKey);
  } catch {
    return defaultPlaybackQuality;
  }
  if (raw === null) return defaultPlaybackQuality;

  try {
    const parsed: unknown = JSON.parse(raw);
    return isTranscodeQuality(parsed) ? parsed : defaultPlaybackQuality;
  } catch {
    return defaultPlaybackQuality;
  }
}

export function writePlaybackQuality(value: PlaybackQuality, storage?: Storage): void {
  try {
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(value));
  } catch {
    // Storage failures must not block the current player's quality selection.
  }
}
