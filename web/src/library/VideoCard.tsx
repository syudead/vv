import { AlertTriangle, Check, ImageOff } from "lucide-react";
import { memo } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { cn } from "../lib/cn";
import {
  formatBytes,
  formatDuration,
  formatRelative,
  qualityLabel,
  unplayableText,
  watchState,
  watchedRatio,
} from "../lib/format";
import Checkbox from "../ui/Checkbox";

export interface VideoCardProps {
  video: Video;
  /** 遷移元の一覧 URL。再生画面の「戻る」がここへ帰る。 */
  backTo: string;
  selected: boolean;
  selectionMode: boolean;
  onSelect: (id: number, selected: boolean) => void;
}

function VideoCard({ video, backTo, selected, selectionMode, onSelect }: VideoCardProps) {
  const duration = formatDuration(video.durationMs);
  const unplayable = unplayableText(video);
  const state = watchState(video);
  const ratio = watchedRatio(video);
  const quality = qualityLabel(video);
  const meta = [quality, video.videoCodec?.toUpperCase(), formatBytes(video.sizeBytes)]
    .filter((part) => part !== undefined && part !== "")
    .join(" · ");

  return (
    <article
      data-video-id={video.id}
      className={cn("group relative flex flex-col", selectionMode && "select-none")}
    >
      <div
        className={cn(
          "absolute top-2 left-2 z-20 transition-opacity duration-150",
          selectionMode || selected
            ? "opacity-100"
            : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
        )}
      >
        <Checkbox
          checked={selected}
          onCheckedChange={(next) => onSelect(video.id, next)}
          label={`「${video.title}」を選択`}
          onClick={(event) => event.stopPropagation()}
        />
      </div>

      <Link
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={video.title}
        onClick={(event) => {
          if (selectionMode) {
            event.preventDefault();
            onSelect(video.id, !selected);
          }
        }}
        className="flex flex-col gap-2.5 rounded-lg outline-offset-4"
      >
        {/* サムネイル枠。16:9 固定で、画像の有無で高さが変わらない。 */}
        <div
          className={cn(
            "relative aspect-video w-full overflow-hidden rounded-lg bg-surface ring-2 transition-[box-shadow,transform] duration-200 ease-out-quart",
            selected ? "ring-accent" : "ring-transparent group-hover:ring-border-strong",
            selectionMode && !selected && "opacity-70",
          )}
        >
          {video.thumbnailUrl !== undefined ? (
            <img
              src={video.thumbnailUrl}
              alt=""
              loading="lazy"
              decoding="async"
              className="h-full w-full object-cover transition-transform duration-300 ease-out-quart group-hover:scale-[1.04]"
            />
          ) : (
            <div className="flex h-full w-full flex-col items-center justify-center gap-1.5 text-fg-subtle">
              <ImageOff className="size-6" strokeWidth={1.5} />
              <span className="text-xs">
                {video.thumbnailState === "failed" ? "画像なし" : "準備中"}
              </span>
            </div>
          )}

          {/* ホバー時のメタ（Stash 的な情報密度）。下端のグラデーションに乗せる。 */}
          {meta !== "" && (
            <div className="pointer-events-none absolute inset-x-0 bottom-0 flex h-16 items-end bg-linear-to-t from-bg/90 via-bg/50 to-transparent px-2.5 pb-2 opacity-0 transition-opacity duration-200 group-hover:opacity-100">
              <span className="pr-14 text-[11px] font-medium text-fg/90 tabular-nums">
                {meta}
              </span>
            </div>
          )}

          {duration !== "" && (
            <span className="absolute right-1.5 bottom-1.5 rounded-sm bg-bg/85 px-1.5 py-0.5 text-[11px] font-semibold text-fg tabular-nums backdrop-blur-sm">
              {duration}
            </span>
          )}

          {state === "watched" && (
            <span className="absolute top-1.5 right-1.5 flex size-6 items-center justify-center rounded-full bg-bg/85 text-success backdrop-blur-sm">
              <Check className="size-3.5" strokeWidth={3} />
              <span className="sr-only">視聴済み</span>
            </span>
          )}

          {ratio !== null && (
            <span
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(ratio * 100)}
              aria-label="再生済みの割合"
              className="absolute inset-x-0 bottom-0 h-1 bg-fg/20"
            >
              <span
                className="block h-full bg-accent"
                style={{ width: `${String(Math.round(ratio * 100))}%` }}
              />
            </span>
          )}

          {unplayable !== null && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 bg-bg/70 text-warning backdrop-blur-[2px]">
              <AlertTriangle className="size-5" />
              <span className="px-3 text-center text-xs font-medium">{unplayable}</span>
            </div>
          )}
        </div>

        <div className="flex min-w-0 flex-col gap-0.5 px-0.5">
          <h3
            title={video.title}
            className={cn(
              "line-clamp-2 text-sm leading-5 font-medium break-all transition-colors group-hover:text-fg",
              state === "watched" ? "text-fg-muted" : "text-fg",
            )}
          >
            {video.title}
          </h3>
          <p className="text-xs text-fg-subtle tabular-nums">
            {formatRelative(video.addedAt)}
          </p>
        </div>
      </Link>
    </article>
  );
}

export default memo(VideoCard);
