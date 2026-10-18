import type { Video } from "../api/client";

export const defaultAspectRatio = 16 / 9;

/** Clamp extreme aspect ratios so controls and status panels fit; letterbox the media within that frame. */
const minAspectRatio = 9 / 16;
const maxAspectRatio = 21 / 9;

export function frameAspectRatio(
  width: number | undefined,
  height: number | undefined,
): number {
  if (width === undefined || height === undefined) return defaultAspectRatio;
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return defaultAspectRatio;
  }
  return Math.min(maxAspectRatio, Math.max(minAspectRatio, width / height));
}

/** Prefer the playback ratio, then probe display ratio with pixel aspect correction, then raw dimensions. */
export function playerAspectRatio(
  video: Pick<Video, "width" | "height" | "displayAspectRatio"> | undefined,
  mediaAspect: number | null,
): number {
  if (mediaAspect !== null) return frameAspectRatio(mediaAspect, 1);
  if (video?.displayAspectRatio !== undefined)
    return frameAspectRatio(video.displayAspectRatio, 1);
  return frameAspectRatio(video?.width, video?.height);
}

/** Use display ratio times height when available; otherwise use pixel width, or no native-width limit. */
export function playerNativeWidth(
  video: Pick<Video, "width" | "height" | "displayAspectRatio"> | undefined,
): number | undefined {
  const width = video?.width;
  const height = video?.height;
  if (height !== undefined && video?.displayAspectRatio !== undefined) {
    const shown = height * video.displayAspectRatio;
    if (Number.isFinite(shown) && shown > 0) return Math.round(shown);
  }
  if (width === undefined || !Number.isFinite(width) || width <= 0) return undefined;
  return width;
}
