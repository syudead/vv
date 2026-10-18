import type { Video } from "../api/client";
import { formatResolution } from "../lib/format";

export type TechnicalSummary =
  { kind: "values"; values: string[] } | { kind: "pending" } | { kind: "failed" };

const codecNames: Record<string, string> = {
  h264: "H.264",
  avc: "H.264",
  avc1: "H.264",
  hevc: "H.265",
  h265: "H.265",
};

export function formatCodec(codec: string): string {
  return codecNames[codec.toLowerCase()] ?? codec.toUpperCase();
}

export function technicalSummary(video: Video): TechnicalSummary {
  if (video.probeState === "failed") return { kind: "failed" };
  if (video.probeState === "pending") return { kind: "pending" };
  const values = [
    formatResolution(video),
    video.container?.toUpperCase(),
    video.videoCodec === undefined ? undefined : formatCodec(video.videoCodec),
    video.audioCodec === undefined ? undefined : formatCodec(video.audioCodec),
  ].filter((value): value is string => value !== undefined && value !== "");
  return { kind: "values", values };
}
