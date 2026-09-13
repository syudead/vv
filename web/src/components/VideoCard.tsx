import { Link } from "react-router";

import type { Video } from "../api/client";

/** formatDuration は尺を mm:ss（1時間以上は h:mm:ss）で表す。 */
export function formatDuration(durationMs: number | undefined): string {
  if (durationMs === undefined || durationMs < 0) {
    return "";
  }

  const totalSeconds = Math.floor(durationMs / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  const pad = (value: number) => String(value).padStart(2, "0");
  if (hours > 0) {
    return `${String(hours)}:${pad(minutes)}:${pad(seconds)}`;
  }
  return `${String(minutes)}:${pad(seconds)}`;
}

/** unplayableText は再生できない理由を、利用者に伝わる言葉にする。 */
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

  switch (video.unplayableReason) {
    case "container":
      return `${video.container ?? "この形式"} は再生できません`;
    case "video_codec":
      return `映像の形式 (${video.videoCodec ?? "不明"}) は再生できません`;
    case "audio_codec":
      return `音声の形式 (${video.audioCodec ?? "不明"}) は再生できません`;
    default:
      return "再生できません";
  }
}

/**
 * partialRatio は途中まで見た割合を返す。見ていない・見終わった・尺が
 * 分からない場合は null を返す（帯を描かない）。
 */
export function partialRatio(video: Video): number | null {
  const progress = video.progress;
  if (progress === undefined || progress.completed) {
    return null;
  }
  if (video.durationMs === undefined || video.durationMs <= 0) {
    return null;
  }
  const ratio = progress.positionMs / video.durationMs;
  if (ratio <= 0) {
    return null;
  }
  return Math.min(ratio, 1);
}

/**
 * VideoCard は一覧の1件を描く。
 *
 * サムネイルは固定アスペクト比（16:9）の枠に入れ、未生成でも枠だけを出す。
 * 画像が届いてからレイアウトが動くと、読んでいる位置が飛ぶ（R-114 / FR-010）。
 */
export default function VideoCard({ video }: { video: Video }) {
  const duration = formatDuration(video.durationMs);
  const unplayable = unplayableText(video);
  const watched = video.progress?.completed === true;
  const watchedRatio = partialRatio(video);

  return (
    <Link
      to={`/videos/${String(video.id)}`}
      className="group flex flex-col gap-2 rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-500"
    >
      <div className="relative aspect-video w-full overflow-hidden rounded-lg bg-neutral-200">
        {video.thumbnailUrl !== undefined ? (
          <img
            src={video.thumbnailUrl}
            alt=""
            loading="lazy"
            className="h-full w-full object-cover transition-opacity group-hover:opacity-90"
          />
        ) : (
          <span className="flex h-full w-full items-center justify-center text-xs text-neutral-500">
            {video.thumbnailState === "failed"
              ? "画像を作れませんでした"
              : "画像を準備中"}
          </span>
        )}

        {duration !== "" && (
          <span className="absolute right-1.5 bottom-1.5 rounded bg-black/75 px-1.5 py-0.5 font-mono text-xs text-white">
            {duration}
          </span>
        )}

        {/* 視聴済みと途中まで見た動画を一覧上で区別できるようにする（FR-015）。
            見終わったものは印で、途中のものは残りの量が分かる帯で示す。 */}
        {watched && (
          <span className="absolute top-1.5 right-1.5 rounded bg-sky-600 px-1.5 py-0.5 text-xs text-white">
            視聴済み
          </span>
        )}

        {!watched && watchedRatio !== null && (
          <span
            aria-label={`${String(Math.round(watchedRatio * 100))}% まで再生済み`}
            className="absolute inset-x-0 bottom-0 h-1 bg-black/40"
          >
            <span
              className="block h-full bg-sky-500"
              style={{ width: `${String(Math.round(watchedRatio * 100))}%` }}
            />
          </span>
        )}

        {unplayable !== null && (
          <span className="absolute top-1.5 left-1.5 rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-900">
            {unplayable}
          </span>
        )}
      </div>

      <h3
        className={`line-clamp-2 text-sm leading-snug font-medium group-hover:underline ${
          watched ? "text-neutral-500" : "text-neutral-900"
        }`}
        title={video.title}
      >
        {video.title}
      </h3>
    </Link>
  );
}
