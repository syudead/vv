import { ImageOff } from "lucide-react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import type { RelatedState } from "../api/useVideoDetail";
import { type HoverPreview, useHoverPreview } from "./useHoverPreview";
import { cn } from "../lib/cn";
import { formatDuration, isNarrowVideo, watchedRatio } from "../lib/format";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import Button from "../ui/Button";
import Skeleton from "../ui/Skeleton";

/**
 * videoLinkLabel は関連動画と「次の動画」のリンクの読み上げ名である。題名と長さだけにし、
 * 中の進捗バーの割合が名前に混ざらないようにする。進捗バー自体は読み上げに残す。
 */
export function videoLinkLabel(video: Video): string {
  const duration = formatDuration(video.durationMs);
  return duration === "" ? video.title : `${video.title} ${duration}`;
}

/**
 * VideoThumbnail は関連動画と「次の動画」のサムネイルである。無いときは一覧のカードと
 * 同じ代わりの表示にし、右下に長さ、途中まで見た動画だけ下端に進捗バーを出す。
 */
export function VideoThumbnail({
  video,
  className,
  preview,
}: {
  video: Video;
  className?: string;
  /** マウスを乗せたときの一覧用プレビュー。関連動画の列だけが渡す。 */
  preview?: HoverPreview;
}) {
  const duration = formatDuration(video.durationMs);
  const ratio = watchedRatio(video);
  return (
    <div
      className={cn(
        "relative aspect-video shrink-0 overflow-hidden rounded-md bg-surface",
        className,
      )}
    >
      {video.thumbnailUrl !== undefined && isNarrowVideo(video) && (
        <ThumbnailBackdrop src={video.thumbnailUrl} />
      )}
      {video.thumbnailUrl !== undefined ? (
        <img
          src={video.thumbnailUrl}
          alt=""
          loading="lazy"
          decoding="async"
          className={cn(
            "relative h-full w-full object-contain",
            preview?.playing === true && "opacity-0",
          )}
        />
      ) : (
        <div className="flex h-full w-full flex-col items-center justify-center gap-1 text-fg-subtle">
          <ImageOff className="size-5" strokeWidth={1.5} aria-hidden="true" />
          <span className="text-xs">
            {video.thumbnailState === "failed" ? "画像なし" : "準備中"}
          </span>
        </div>
      )}
      {preview?.active === true && preview.previewUrl !== undefined && (
        <video
          ref={preview.videoRef}
          src={preview.previewUrl}
          muted
          loop
          playsInline
          preload="auto"
          controls={false}
          aria-hidden="true"
          tabIndex={-1}
          onPlaying={preview.onPlaying}
          onError={preview.onError}
          className={cn(
            "absolute inset-0 h-full w-full object-contain",
            preview.playing ? "opacity-100" : "opacity-0",
          )}
        />
      )}
      {duration !== "" && (
        <span className="absolute right-1 bottom-1 rounded-sm bg-overlay px-1 text-xs text-fg tabular-nums">
          {duration}
        </span>
      )}
      {ratio !== null && (
        <span
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(ratio * 100)}
          aria-label="再生済みの割合"
          className="absolute inset-x-0 bottom-0 h-[3px] bg-fg-subtle/50"
        >
          <span
            className="block h-full bg-accent"
            style={{ width: `${String(Math.round(ratio * 100))}%` }}
          />
        </span>
      )}
    </div>
  );
}

/**
 * RelatedVideos は関連動画の列である（要件 15）。
 *
 * 関連動画が 0 件のときは見出しと並びを出さない（閉じる × は見出しの帯 VideoHeader にある）。
 * 各項目はサムネイル・長さ・題名だけで、追加日時などの文字は出さない。
 * リンクは最初の戻り先を引き継ぐ（plan の Structural Decisions 10）。
 */
export default function RelatedVideos({
  state,
  backTo,
  onRetry,
}: {
  state: RelatedState;
  backTo: string;
  onRetry: () => void;
}) {
  const empty = state.kind === "ready" && state.related.items.length === 0;
  return (
    <section
      aria-labelledby={empty ? undefined : "related-heading"}
      className="flex flex-col gap-3 lg:min-h-0 lg:flex-1"
    >
      {!empty && (
        <h2 id="related-heading" className="text-sm font-semibold text-fg">
          関連動画
        </h2>
      )}

      {state.kind === "loading" && (
        <ul aria-hidden="true" className="flex flex-col gap-3">
          {Array.from({ length: 6 }, (_, index) => (
            <li key={index} className="flex gap-3">
              <Skeleton className="aspect-video w-40 shrink-0" />
              <div className="flex flex-1 flex-col gap-2 pt-1">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            </li>
          ))}
        </ul>
      )}

      {state.kind === "failed" && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-sm text-fg-muted">関連動画を取得できませんでした</p>
          <Button variant="ghost" size="sm" onClick={onRetry}>
            再試行
          </Button>
        </div>
      )}

      {state.kind === "ready" && !empty && (
        <ul
          // 広い画面では並びだけを中でスクロールさせ、端まで来てもページへは送らない。
          // 行の hover の面と輪郭が切れないよう、はみ出す分だけ内側に余白を取る。
          className="flex flex-col gap-3 lg:-mx-1.5 lg:min-h-0 lg:scrollbar-on-hover lg:overflow-y-auto lg:overscroll-contain lg:px-1.5 lg:pt-1.5 lg:pb-6"
        >
          {state.related.items.map((video) => (
            <RelatedItem key={video.id} video={video} backTo={backTo} />
          ))}
        </ul>
      )}
    </section>
  );
}

function RelatedItem({ video, backTo }: { video: Video; backTo: string }) {
  const preview = useHoverPreview(video);
  return (
    <li>
      <Link
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={videoLinkLabel(video)}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={preview.onPointerLeave}
        className="-m-1.5 flex gap-3 rounded-lg p-1.5 transition-colors hover:bg-hover-wash"
      >
        <VideoThumbnail video={video} className="w-40" preview={preview} />
        <span className="line-clamp-2 min-w-0 text-sm font-medium text-fg [overflow-wrap:anywhere]">
          {video.title}
        </span>
      </Link>
    </li>
  );
}
