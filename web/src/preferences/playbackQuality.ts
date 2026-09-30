import { isTranscodeQuality, type TranscodeQuality } from "../api/client";

/**
 * PlaybackQuality は見る人が選ぶ再生の画質である。"original" は元の画質で、それ以外は
 * ライブ変換で縮める画質（specs/027-playback-quality/research.md R-3・R-5）。
 */
export type PlaybackQuality = "original" | TranscodeQuality;

const storageKey = "vv.playback-quality.v1";

export const defaultPlaybackQuality: PlaybackQuality = "original";

/** 保存値が無い・壊れている・読めない場合は「元の画質」を返す。 */
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

/** 画質をこのブラウザに保存する。保存領域が使えなくても再生は妨げない。 */
export function writePlaybackQuality(value: PlaybackQuality, storage?: Storage): void {
  try {
    (storage ?? window.localStorage).setItem(storageKey, JSON.stringify(value));
  } catch {
    // プライベートモードなどで保存できなくても、今のプレイヤーでは選んだ画質を使える。
  }
}
