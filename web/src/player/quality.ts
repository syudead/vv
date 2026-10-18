import { transcodeQualities, type TranscodeQuality, type Video } from "../api/client";
import type { PlaybackQuality } from "../preferences/playbackQuality";

const qualityShortSide: Record<TranscodeQuality, number> = {
  "1080p": 1080,
  "720p": 720,
  "480p": 480,
  "360p": 360,
};

/** Offer qualities below the source's short side, matching the server's TranscodeQuality.Available rule. */
export function qualityOptions(
  video: Pick<Video, "width" | "height">,
): TranscodeQuality[] {
  const { width, height } = video;
  if (width === undefined || height === undefined || width <= 0 || height <= 0) return [];
  const shortSide = Math.min(width, height);
  return transcodeQualities.filter((quality) => qualityShortSide[quality] < shortSide);
}

/** Fall back to original when a saved quality is unavailable without overwriting that preference. */
export function effectiveQuality(
  remembered: PlaybackQuality,
  video: Pick<Video, "width" | "height">,
): PlaybackQuality {
  if (remembered === "original") return remembered;
  return qualityOptions(video).includes(remembered) ? remembered : "original";
}

export function sourceShortSide(
  video: Pick<Video, "width" | "height">,
): string | undefined {
  const { width, height } = video;
  if (width === undefined || height === undefined || width <= 0 || height <= 0) {
    return undefined;
  }
  return `${String(Math.min(width, height))}p`;
}
