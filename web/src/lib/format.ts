import type { Video } from "../api/client";

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

/** formatRelative は「3 日前」のような相対表記。now は検査のために差し替えられる。 */
export function formatRelative(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) {
    return "";
  }
  const diffSec = Math.round((now.getTime() - then.getTime()) / 1000);
  if (diffSec < 45) return "たった今";
  const minutes = Math.round(diffSec / 60);
  if (minutes < 60) return `${String(minutes)} 分前`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${String(hours)} 時間前`;
  const days = Math.round(hours / 24);
  if (days < 7) return `${String(days)} 日前`;
  const weeks = Math.round(days / 7);
  if (weeks < 5) return `${String(weeks)} 週間前`;
  const months = Math.round(days / 30);
  if (months < 12) return `${String(months)} か月前`;
  const years = Math.round(days / 365);
  return `${String(years)} 年前`;
}

/** formatDateTime は絶対日時を日本語ロケールで表す。 */
export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleString("ja-JP", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** unplayableText は解析中または解析失敗を利用者に伝える。再生を試せる動画は null。 */
export function unplayableText(video: Video): string | null {
  if (video.playable) {
    return null;
  }
  if (video.probeState === "failed") {
    return "読み取れませんでした";
  }
  if (video.probeState === "pending") {
    return "確認中";
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
