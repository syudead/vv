import { useSortable } from "@dnd-kit/sortable";
import { AlertTriangle, Check, Folder, GripVertical, X } from "lucide-react";
import type { Ref } from "react";
import { Link } from "react-router";

import type { PlaylistItem } from "../api/playlists";
import { formatNumber, t } from "../i18n";
import { cn } from "../lib/cn";
import {
  formatDuration,
  isNarrowVideo,
  unplayableText,
  watchedRatio,
  watchState,
} from "../lib/format";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import { itemDurationMs, itemTitle, playTarget } from "./playlistItems";

interface RowFacts {
  thumbnailUrl: string | undefined;
  narrow: boolean;
  state: "unwatched" | "inProgress" | "watched";
  /** 進み具合の棒の割合（0〜1）。見ている途中でなければ null。 */
  ratio: number | null;
  /** グループの「3 / 12」。グループでなければ null。 */
  groupProgress: string | null;
  /** グループの 2 行目（「12 videos」）。 */
  groupCount: string | null;
  unplayable: string | null;
}

/** rowFacts は行に出す値を、動画とグループで同じ形にそろえる（library のリスト表示と同じ値）。 */
function rowFacts(item: PlaylistItem): RowFacts {
  const group = item.group;
  if (item.kind === "group" && group !== undefined) {
    const watched = group.watchedCount ?? 0;
    const state = group.watchState ?? "unwatched";
    return {
      thumbnailUrl: group.previews[0]?.thumbnailUrl,
      narrow: false,
      state,
      ratio:
        state === "inProgress" && group.videoCount > 0
          ? Math.min(watched / group.videoCount, 1)
          : null,
      groupProgress:
        state === "inProgress"
          ? String(t.library.group.progress(watched, group.videoCount))
          : null,
      groupCount: String(t.library.group.count(group.videoCount)),
      unplayable: null,
    };
  }
  const video = item.video;
  if (video === undefined) {
    return {
      thumbnailUrl: undefined,
      narrow: false,
      state: "unwatched",
      ratio: null,
      groupProgress: null,
      groupCount: null,
      unplayable: null,
    };
  }
  const unplayable = unplayableText(video);
  return {
    thumbnailUrl: video.thumbnailUrl,
    narrow: isNarrowVideo(video),
    state: watchState(video),
    ratio: watchedRatio(video),
    groupProgress: null,
    groupCount: null,
    unplayable: unplayable === null ? null : String(unplayable),
  };
}

/**
 * PlaylistItemRow はプレイリストの画面の 1 行である（specs/044-playlists/ui-design.md「Rows」）。
 * ライブラリのリスト表示の行と同じ列と高さに、前へハンドルと番号、後ろへ外す `×` を足す。
 *
 * 行のリンク（サムネイルと題名）はその項目から再生を始める。行はハンドルからだけ持ち上がり
 * （research.md R-9）、持ち上げている間はリンクを押せない。
 */
export default function PlaylistItemRow({
  playlistId,
  item,
  position,
  removing,
  onRemove,
  removeRef,
}: {
  playlistId: number;
  item: PlaylistItem;
  /** 1 始まりの番号。 */
  position: number;
  /** 外す要求を送っている間 true。 */
  removing: boolean;
  onRemove: (item: PlaylistItem) => void;
  removeRef: Ref<HTMLButtonElement>;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id });
  const title = itemTitle(item);
  const facts = rowFacts(item);
  const target = playTarget(playlistId, item);
  const duration = formatDuration(itemDurationMs(item));

  return (
    <TableRow
      ref={setNodeRef}
      data-item-id={item.id}
      // 位置は持ち上げている間だけ実行時に決まる値なので style で渡す。横には動かさない。
      style={
        transform === null
          ? { transition }
          : { transform: `translate3d(0, ${String(transform.y)}px, 0)`, transition }
      }
      className={cn(
        "relative motion-reduce:transition-none",
        isDragging && "z-10 bg-popover shadow-elevated hover:bg-popover",
      )}
    >
      <TableCell className="w-0 pr-0 pl-1">
        <span className="flex items-center gap-1">
          <Button
            ref={setActivatorNodeRef}
            type="button"
            variant="ghost"
            size="icon-sm"
            className="relative z-10 cursor-grab touch-none text-muted-foreground"
            {...attributes}
            {...listeners}
            aria-label={t.playlists.item.reorder(title)}
          >
            <GripVertical aria-hidden="true" />
          </Button>
          <span className="min-w-4 text-right text-xs text-muted-foreground tabular-nums">
            {formatNumber(position)}
          </span>
        </span>
      </TableCell>
      <TableCell className="w-list-thumb-cell">
        <VideoThumbnail className="w-list-thumb rounded-sm">
          {facts.thumbnailUrl !== undefined && facts.narrow && (
            <ThumbnailBackdrop src={facts.thumbnailUrl} />
          )}
          {facts.thumbnailUrl !== undefined && (
            <VideoThumbnailImage src={facts.thumbnailUrl} />
          )}
          {duration !== "" && <VideoThumbnailDuration>{duration}</VideoThumbnailDuration>}
          {facts.ratio !== null && (
            <VideoThumbnailProgress
              value={Math.round(facts.ratio * 100)}
              aria-label={
                item.kind === "group"
                  ? t.library.group.watchedRatio
                  : t.list.card.watchedRatio
              }
            />
          )}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="w-full max-w-0 pr-2 whitespace-normal sm:pr-4">
        {target === null ? (
          <span title={title} className="line-clamp-2 text-sm font-medium break-all">
            {title}
          </span>
        ) : (
          // リンクの after: を行（relative）いっぱいに広げ、行のどこを押しても再生を始める。
          // ハンドルと × は z-10 でその上に置く。
          <Link
            to={target.to}
            state={target.state}
            title={title}
            aria-label={t.playlists.item.playFrom(title)}
            className={cn(
              "line-clamp-2 rounded-sm text-sm font-medium break-all after:absolute after:inset-0 hover:text-primary",
              facts.state === "watched" ? "text-muted-foreground" : "text-foreground",
              isDragging && "pointer-events-none",
            )}
          >
            {title}
          </Link>
        )}
        {facts.groupCount !== null && (
          <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
            <Folder aria-hidden="true" className="size-3 shrink-0" />
            {facts.groupCount}
          </span>
        )}
        {facts.unplayable !== null && (
          <span className="mt-0.5 flex items-center gap-1 text-xs text-warning">
            <AlertTriangle aria-hidden="true" className="size-3 shrink-0" />
            {facts.unplayable}
          </span>
        )}
      </TableCell>
      <TableCell className="hidden w-16 pr-4 text-right text-xs text-muted-foreground tabular-nums sm:table-cell">
        {facts.state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
        {facts.groupProgress}
      </TableCell>
      {/* 狭い幅ではサムネイルの長さだけを出し、題名の幅を残す。 */}
      <TableCell className="hidden w-0 pr-2 text-right tabular-nums sm:table-cell">
        {duration}
      </TableCell>
      <TableCell className="w-0 pr-1 pl-0 sm:pr-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              ref={removeRef}
              type="button"
              variant="ghost"
              size="icon-sm"
              className="relative z-10 text-muted-foreground"
              disabled={removing}
              aria-busy={removing || undefined}
              aria-label={t.playlists.item.removeFor(title)}
              onClick={() => onRemove(item)}
            >
              {removing ? <Spinner aria-hidden="true" /> : <X aria-hidden="true" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t.playlists.item.remove}</TooltipContent>
        </Tooltip>
      </TableCell>
    </TableRow>
  );
}
