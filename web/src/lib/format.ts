import type { Video } from "../api/client";
import { t, type UiText } from "../i18n";

// ロケールに依存しない書式（長さ、容量、解像度）である。日時・相対時刻・数は i18n/ にある。

/** formatDuration は尺を m:ss（1 時間以上は h:mm:ss）で表す。不明なら空。 */
export function formatDuration(durationMs: number | undefined): string {
  if (durationMs === undefined || durationMs < 0) {
    return "";
  }
  const total = Math.floor(durationMs / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${String(h)}:${pad(m)}:${pad(s)}` : `${String(m)}:${pad(s)}`;
}

/** formatBytes は大きさを読みやすい単位で表す（1024 進）。 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return "";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  const digits = index === 0 ? 0 : value >= 100 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[index]}`;
}

/** formatResolution は 1920×1080 のように表す。片方でも欠けていれば空。 */
export function formatResolution(video: Pick<Video, "width" | "height">): string {
  if (video.width === undefined || video.height === undefined) {
    return "";
  }
  return `${String(video.width)}×${String(video.height)}`;
}

/** qualityLabel は 4K / 1080p / 720p のような短い品質表記。 */
export function qualityLabel(video: Pick<Video, "width" | "height">): string {
  const h = video.height;
  const w = video.width;
  if (h === undefined || w === undefined) {
    return "";
  }
  const shorter = Math.min(w, h);
  if (shorter >= 2160) return "4K";
  if (shorter >= 1440) return "1440p";
  if (shorter >= 1080) return "1080p";
  if (shorter >= 720) return "720p";
  if (shorter >= 480) return "480p";
  return `${String(shorter)}p`;
}

/** unplayableText は解析中または解析失敗を利用者に伝える。再生を試せる動画は null。 */
export function unplayableText(video: Video): UiText | null {
  if (video.playable) {
    return null;
  }
  if (video.probeState === "failed") {
    return t.video.unplayable.failed;
  }
  if (video.probeState === "pending") {
    return t.video.unplayable.pending;
  }
  if (
    video.durationMs === undefined ||
    video.durationMs <= 0 ||
    video.videoCodec === undefined
  ) {
    return t.video.unplayable.missingInfo;
  }
  return null;
}

export type WatchState = "unwatched" | "inProgress" | "watched";

/** watchState は視聴状態を 3 値に畳む。 */
export function watchState(video: Pick<Video, "progress">): WatchState {
  const p = video.progress;
  if (p === undefined) return "unwatched";
  if (p.completed) return "watched";
  return p.positionMs > 0 ? "inProgress" : "unwatched";
}

/** watchedRatio は途中まで見た割合（0..1）。途中でなければ null。 */
export function watchedRatio(
  video: Pick<Video, "progress" | "durationMs">,
): number | null {
  if (watchState(video) !== "inProgress") return null;
  if (video.durationMs === undefined || video.durationMs <= 0) return null;
  const ratio = (video.progress?.positionMs ?? 0) / video.durationMs;
  return ratio <= 0 ? null : Math.min(ratio, 1);
}

/**
 * isNarrowVideo は、16:9 の枠に入れると左右の余白が大きく目立つ動画（縦長・正方形に近いもの）
 * かどうかを返す。表示の比率が分からない既存の動画は解像度の比で判断し、どちらも無ければ偽。
 */
export function isNarrowVideo(
  video: Pick<Video, "width" | "height" | "displayAspectRatio">,
): boolean {
  const ratio =
    video.displayAspectRatio ??
    (video.width !== undefined && video.height !== undefined && video.height > 0
      ? video.width / video.height
      : undefined);
  return ratio !== undefined && ratio > 0 && ratio < 1.25;
}
