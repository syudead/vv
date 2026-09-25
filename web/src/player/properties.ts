import type { Video } from "../api/client";
import { formatBytes, formatDateTime, formatResolution } from "../lib/format";

/**
 * 題名の下の属性の一列に並べる値（ui-design「Property strip and file location」）。
 *
 * ラベルは英語の大文字、値は大文字の表記にそろえる。読み取り前・読み取り失敗・音声なしの
 * 書き方もここで決める。
 */

export interface Property {
  label: string;
  value: string;
  /** muted は控えめな色（読み取り中・未再生）、warning は読み取り失敗。 */
  tone?: "muted" | "warning";
}

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

const missing = "—";

/**
 * videoProperties は属性の一列の項目を、表示する順に返す。`lastPlayed` が偽なら
 * LAST PLAYED を抜く（ゲストの再生画面。specs/016-single-account-auth/ui-design.md
 * 「Guest degradation」）。
 */
export function videoProperties(
  video: Video,
  { lastPlayed = true }: { lastPlayed?: boolean } = {},
): Property[] {
  const technical: Property[] = (() => {
    if (video.probeState === "failed") {
      return [{ label: "MEDIA", value: "読み取れませんでした", tone: "warning" }];
    }
    if (video.probeState === "pending") {
      return ["RESOLUTION", "CONTAINER", "VIDEO", "AUDIO"].map((label) => ({
        label,
        value: "読み取り中",
        tone: "muted",
      }));
    }
    const resolution = formatResolution(video);
    return [
      { label: "RESOLUTION", value: resolution === "" ? missing : resolution },
      {
        label: "CONTAINER",
        value: video.container === undefined ? missing : video.container.toUpperCase(),
      },
      {
        label: "VIDEO",
        value: video.videoCodec === undefined ? missing : formatCodec(video.videoCodec),
      },
      {
        label: "AUDIO",
        value: video.audioCodec === undefined ? missing : formatCodec(video.audioCodec),
      },
    ];
  })();

  const common: Property[] = [
    ...technical,
    { label: "SIZE", value: formatBytes(video.sizeBytes) },
    { label: "ADDED", value: formatDate(video.addedAt) },
  ];
  if (!lastPlayed) return common;
  return [
    ...common,
    video.progress === undefined
      ? { label: "LAST PLAYED", value: missing, tone: "muted" }
      : { label: "LAST PLAYED", value: formatDateTime(video.progress.updatedAt) },
  ];
}

/** splitPath はファイルの場所を、フォルダ部分（区切りを含む）とファイル名に分ける。 */
export function splitPath(path: string): { folder: string; name: string } {
  const index = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return { folder: path.slice(0, index + 1), name: path.slice(index + 1) };
}
