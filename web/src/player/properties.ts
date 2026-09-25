import type { Video } from "../api/client";
import { formatResolution } from "../lib/format";

/**
 * 題名の下の技術情報の行に並べる値（ui-design「Video facts」）。
 *
 * 解像度・コンテナ・映像・音声の順に、大文字の表記で並べる。分からない値は並べない。
 * 読み取り前・読み取り失敗は、値の代わりにその状態を 1 つだけ返す。
 */
export type TechnicalSummary =
  { kind: "values"; values: string[] } | { kind: "pending" } | { kind: "failed" };

const codecNames: Record<string, string> = {
  h264: "H.264",
  avc: "H.264",
  avc1: "H.264",
  hevc: "H.265",
  h265: "H.265",
};

/** formatCodec は `h264` を `H.264`、`aac` を `AAC` のように書く。 */
export function formatCodec(codec: string): string {
  return codecNames[codec.toLowerCase()] ?? codec.toUpperCase();
}

/** formatDate は日付だけを `2026/09/20` の形で書く。 */
export function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("ja-JP", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

/** technicalSummary は技術情報の行の中身を返す。 */
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
