export interface PlaybackVolume {
  volume: number;
  muted: boolean;
}

const storageKey = "vv.playback-volume.v1";

export const defaultPlaybackVolume: PlaybackVolume = { volume: 1, muted: false };

/** Return safe defaults when storage is absent, invalid or unreadable. */
export function readPlaybackVolume(storage?: Storage): PlaybackVolume {
  let raw: string | null;
  try {
    raw = (storage ?? window.localStorage).getItem(storageKey);
  } catch {
    return { ...defaultPlaybackVolume };
  }
  if (raw === null) return { ...defaultPlaybackVolume };

  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return { ...defaultPlaybackVolume };
    }
    const value = parsed as Record<string, unknown>;
    return {
      volume:
        typeof value.volume === "number" &&
        Number.isFinite(value.volume) &&
        value.volume >= 0 &&
        value.volume <= 1
          ? value.volume
          : defaultPlaybackVolume.volume,
      muted: typeof value.muted === "boolean" ? value.muted : defaultPlaybackVolume.muted,
    };
  } catch {
    return { ...defaultPlaybackVolume };
  }
}

export function writePlaybackVolume(value: PlaybackVolume, storage?: Storage): void {
  try {
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(value));
  } catch {
    // Storage failures must not block the current player's controls.
  }
}
