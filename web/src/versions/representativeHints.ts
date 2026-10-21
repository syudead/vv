import type { Video } from "../api/client";
import { t } from "../i18n";

/** A file this much larger than the other, with no more pixels, is called out. */
const largerRatio = 1.25;
/** Audio this large a share of the file is called out, as lossless audio inflates a file. */
const audioShare = 0.3;
/** Smaller gaps are encoder padding, not something to choose by. */
const startGapMs = 500;
const lengthGapMs = 1000;

function pixels(video: Video): number | undefined {
  return video.width === undefined || video.height === undefined
    ? undefined
    : video.width * video.height;
}

function sizeHints(video: Video, other: Video): string[] {
  const hints = t.versions.duplicates.hints;
  const out: string[] = [];
  const mine = pixels(video);
  const theirs = pixels(other);
  if (
    mine !== undefined &&
    theirs !== undefined &&
    mine <= theirs &&
    other.sizeBytes > 0 &&
    video.sizeBytes >= other.sizeBytes * largerRatio
  )
    out.push(hints.largerNoMorePixels(video.sizeBytes / other.sizeBytes));
  const sizes = video.mediaInfo?.container?.sizes;
  if (
    sizes?.audioBytes !== undefined &&
    sizes.totalBytes !== undefined &&
    sizes.totalBytes > 0 &&
    sizes.audioBytes / sizes.totalBytes >= audioShare
  )
    out.push(hints.audioShare(sizes.audioBytes / sizes.totalBytes));
  return out;
}

function audioHints(video: Video): string[] {
  const hints = t.versions.duplicates.hints;
  const media = video.mediaInfo;
  const audio = media?.audioTracks?.[0];
  const picture = media?.video;
  if (audio === undefined || picture === undefined) return [];
  const out: string[] = [];
  if (audio.startMs !== undefined && picture.startMs !== undefined) {
    const gap = audio.startMs - picture.startMs;
    if (gap >= startGapMs) out.push(hints.audioLate(gap));
    else if (gap <= -startGapMs) out.push(hints.audioEarly(-gap));
  }
  if (audio.durationMs !== undefined && picture.durationMs !== undefined) {
    const gap = picture.durationMs - audio.durationMs;
    if (gap >= lengthGapMs) out.push(hints.audioShorter(gap));
    else if (gap <= -lengthGapMs) out.push(hints.audioLonger(-gap));
  }
  return out;
}

/** What tells this file apart from the other when choosing which to show for both. */
export function representativeHints(video: Video, other: Video): string[] {
  const hints = t.versions.duplicates.hints;
  return [
    ...(video.probeState === "done" && !video.playable ? [hints.unplayable] : []),
    ...(video.mediaInfo?.container?.encrypted === true ? [hints.encrypted] : []),
    ...sizeHints(video, other),
    ...audioHints(video),
  ];
}
