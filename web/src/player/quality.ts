import { transcodeQualities, type TranscodeQuality, type Video } from "../api/client";
import type { PlaybackQuality } from "../preferences/playbackQuality";

/** qualityShortSide は画質ごとの表示の短辺である（contracts/transcode-quality-api.md §2）。 */
const qualityShortSide: Record<TranscodeQuality, number> = {
  "1080p": 1080,
  "720p": 720,
  "480p": 480,
  "360p": 360,
};

/**
 * qualityOptions は動画に選べる縮めた画質を大きい順に返す。動画の表示の短辺
 * （`width`・`height` の小さい方）より小さい画質だけで、寸法の無い動画は空である
 * （親 Issue 要件 1、research.md R-3）。規則は短辺だけで決め、変換の枠による縮小は考えない。
 * サーバーの `TranscodeQuality.Available` と同じ規則である。
 */
export function qualityOptions(
  video: Pick<Video, "width" | "height">,
): TranscodeQuality[] {
  const { width, height } = video;
  if (width === undefined || height === undefined || width <= 0 || height <= 0) return [];
  const shortSide = Math.min(width, height);
  return transcodeQualities.filter((quality) => qualityShortSide[quality] < shortSide);
}

/**
 * effectiveQuality は覚えている画質で、この動画を再生する画質を返す。選択肢に無ければ
 * 元の画質で再生する。覚えている値は書き換えない（Edge Case 2）。
 */
export function effectiveQuality(
  remembered: PlaybackQuality,
  video: Pick<Video, "width" | "height">,
): PlaybackQuality {
  if (remembered === "original") return remembered;
  return qualityOptions(video).includes(remembered) ? remembered : "original";
}
