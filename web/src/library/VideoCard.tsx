import { AlertTriangle, Check, ImageOff } from "lucide-react";
import { memo, type MouseEvent } from "react";
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
  /** 選択を持たない画面（フォルダ画面）では省き、チェックを描かない。 */
  onSelect?: (id: number, selected: boolean) => void;
}

function useCardState(video: Video) {
  return {
    duration: formatDuration(video.durationMs),
    unplayable: unplayableText(video),
    state: watchState(video),
    ratio: watchedRatio(video),
    quality: qualityLabel(video),
  };
}

function SelectCheck({
  video,
  selected,
  selectionMode,
  onSelect,
}: Pick<VideoCardProps, "video" | "selected" | "selectionMode"> & {
  onSelect: (id: number, selected: boolean) => void;
}) {
  return (
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
        onClick={(event: MouseEvent) => event.stopPropagation()}
      />
    </div>
  );
}

/**
 * VideoCard は Stash の scene-card と同じ箱型。サムネイルはカードの端まで、
 * 右下に「720P 59:11」の文字（ホバーで消える）、下に題名と日付・大きさ。
 */
function VideoCard(props: VideoCardProps) {
  const { video, backTo, selected, selectionMode, onSelect } = props;
  const { duration, unplayable, state, ratio, quality } = useCardState(video);

  return (
    <article
      data-video-id={video.id}
      className={cn(
        "group relative flex flex-col overflow-hidden rounded-lg bg-surface shadow-card transition-[box-shadow,transform] duration-200 ease-out-quart",
        // リンクの輪郭は overflow-hidden で切れるので、キーボードフォーカスは箱の外側に出す。
        "has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-link",
        "hover:-translate-y-0.5",
        "hover:shadow-card-hover",
        selected && "ring-2 ring-accent",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
        <SelectCheck
          video={video}
          selected={selected}
          selectionMode={selectionMode}
          onSelect={onSelect}
        />
      )}

      <Link
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={video.title}
        onClick={(event) => {
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(video.id, !selected);
          }
        }}
        className="flex min-w-0 flex-col outline-none"
      >
        <div className="relative aspect-video w-full overflow-hidden bg-navbar">
          {video.thumbnailUrl !== undefined ? (
            <img
              src={video.thumbnailUrl}
              alt=""
              loading="lazy"
              decoding="async"
              className="h-full w-full object-cover object-top transition-transform duration-300 ease-out-quart group-hover:scale-[1.03]"
            />
          ) : (
            <div className="flex h-full w-full flex-col items-center justify-center gap-1.5 text-fg-subtle">
              <ImageOff className="size-6" strokeWidth={1.5} />
              <span className="text-xs">
                {video.thumbnailState === "failed" ? "画像なし" : "準備中"}
              </span>
            </div>
          )}

          {(quality !== "" || duration !== "") && (
            <span className="absolute right-2 bottom-2 flex items-center gap-1.5 rounded-sm bg-navbar/85 px-1.5 py-0.5 text-[11px] font-medium text-fg tabular-nums backdrop-blur-sm">
              {quality !== "" && <span className="text-accent">{quality}</span>}
              {duration !== "" && <span>{duration}</span>}
            </span>
          )}

          {state === "watched" && (
            <span className="absolute top-2 right-2 flex size-6 items-center justify-center rounded-full bg-navbar/85 text-success backdrop-blur-sm">
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
              className="absolute inset-x-0 bottom-0 h-[5px] bg-fg-subtle/50"
            >
              <span
                className="block h-full bg-accent"
                style={{ width: `${String(Math.round(ratio * 100))}%` }}
              />
            </span>
          )}

          {unplayable !== null && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 bg-overlay text-warning">
              <AlertTriangle className="size-5" />
              <span className="px-3 text-center text-xs font-medium">{unplayable}</span>
            </div>
          )}
        </div>

        <div className="flex min-w-0 flex-col gap-1 px-3 pt-2 pb-3">
          <h3
            title={video.title}
            className={cn(
              "line-clamp-2 text-sm leading-5 font-medium break-all",
              state === "watched" ? "text-fg-muted" : "text-fg",
            )}
          >
            {video.title}
          </h3>
          <p className="flex items-center gap-2 text-xs text-fg-muted tabular-nums">
            <span>{formatRelative(video.addedAt)}</span>
            <span className="text-fg-subtle">·</span>
            <span>{formatBytes(video.sizeBytes)}</span>
            {video.videoCodec !== undefined && (
              <>
                <span className="text-fg-subtle">·</span>
                <span className="uppercase">{video.videoCodec}</span>
              </>
            )}
          </p>
        </div>
      </Link>
    </article>
  );
}

export default memo(VideoCard);

/** VideoRow はリスト表示の 1 行。 */
export const VideoRow = memo(function VideoRow(props: VideoCardProps) {
  const { video, backTo, selected, selectionMode, onSelect } = props;
  const { duration, unplayable, state, ratio, quality } = useCardState(video);

  return (
    <tr
      data-video-id={video.id}
      className={cn(
        "group relative transition-colors hover:bg-hover-wash",
        selected && "bg-accent-soft",
      )}
    >
      <td className="w-10 pl-3">
        <Checkbox
          checked={selected}
          onCheckedChange={(next) => onSelect?.(video.id, next)}
          label={`「${video.title}」を選択`}
          className={cn(
            "transition-opacity",
            selectionMode || selected
              ? "opacity-100"
              : "opacity-40 group-hover:opacity-100",
          )}
        />
      </td>
      <td className="w-32 py-1.5 pr-2">
        <div className="relative aspect-video w-28 overflow-hidden rounded-sm bg-navbar">
          {video.thumbnailUrl !== undefined && (
            <img
              src={video.thumbnailUrl}
              alt=""
              loading="lazy"
              decoding="async"
              className="h-full w-full object-cover object-top"
            />
          )}
          {ratio !== null && (
            <span className="absolute inset-x-0 bottom-0 h-[3px] bg-fg-subtle/50">
              <span
                className="block h-full bg-accent"
                style={{ width: `${String(Math.round(ratio * 100))}%` }}
              />
            </span>
          )}
        </div>
      </td>
      <td className="min-w-0 py-1.5 pr-4">
        <Link
          to={`/videos/${String(video.id)}`}
          state={{ from: backTo }}
          onClick={(event) => {
            if (selectionMode) {
              event.preventDefault();
              onSelect?.(video.id, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 text-sm font-medium break-all hover:text-link",
            state === "watched" ? "text-fg-muted" : "text-fg",
          )}
        >
          {video.title}
        </Link>
        {unplayable !== null && (
          <span className="mt-0.5 flex items-center gap-1 text-xs text-warning">
            <AlertTriangle className="size-3" />
            {unplayable}
          </span>
        )}
      </td>
      <td className="hidden w-16 pr-4 text-right text-xs text-fg-muted tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check className="ml-auto size-4 text-success" aria-label="視聴済み" />
        )}
      </td>
      <td className="w-20 pr-4 text-right text-sm text-fg tabular-nums">{duration}</td>
      <td className="hidden w-20 pr-4 text-right text-sm font-bold text-fg uppercase md:table-cell">
        {quality}
      </td>
      <td className="hidden w-24 pr-4 text-right text-sm text-fg-muted tabular-nums md:table-cell">
        {formatBytes(video.sizeBytes)}
      </td>
      <td className="hidden w-28 pr-3 text-right text-sm text-fg-muted tabular-nums lg:table-cell">
        {formatRelative(video.addedAt)}
      </td>
    </tr>
  );
});
