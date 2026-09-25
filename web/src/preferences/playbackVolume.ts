export interface PlaybackVolume {
  volume: number;
  muted: boolean;
}

const storageKey = "vv.playback-volume.v1";

export const defaultPlaybackVolume: PlaybackVolume = { volume: 1, muted: false };

/** 保存値が無い・壊れている・読めない場合も、安全な既定値を返す。 */
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

/** 音量をこのブラウザに保存する。保存領域が使えなくても再生操作は妨げない。 */
export function writePlaybackVolume(value: PlaybackVolume, storage?: Storage): void {
  try {
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(value));
  } catch {
    // プライベートモードなどで保存できなくても、現在のプレイヤーでは設定を使える。
  }
}
