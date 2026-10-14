import { AlertTriangle, Check, Folder } from "lucide-react";

import type { LibraryGroup, Video } from "../api/client";
import { t } from "../i18n";
import {
  formatDuration,
  isNarrowVideo,
  unplayableText,
  watchedRatio,
  watchState,
} from "../lib/format";
import { TableCell } from "../ui/shadcn/table";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";

// 順に並ぶ項目（動画かグループ）の行の共通の列である。プレイリストの画面の行
// （specs/044-playlists/ui-design.md「Rows」）と、それを使うグループのページのリスト表示
// （specs/048-groups/ui-design.md「Body」）が、サムネイル・題名の下の行・視聴済み・長さを
// ここから描く。

/** OrderedItem は順に並ぶ項目である。プレイリストとグループの項目が同じ形を持つ。 */
export interface OrderedItem {
  kind: "video" | "group";
  video?: Video;
  group?: LibraryGroup;
}

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

const emptyFacts: RowFacts = {
  thumbnailUrl: undefined,
  narrow: false,
  state: "unwatched",
  ratio: null,
  groupProgress: null,
  groupCount: null,
  unplayable: null,
};

/** rowFacts は行に出す値を、動画とグループで同じ形にそろえる（library のリスト表示と同じ値）。 */
export function rowFacts(item: OrderedItem): RowFacts {
  const group = item.group;
  if (item.kind === "group" && group !== undefined) {
    const watched = group.watchedCount ?? 0;
    const state = group.watchState ?? "unwatched";
    return {
      ...emptyFacts,
      thumbnailUrl: group.previews[0]?.thumbnailUrl,
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
    };
  }
  const video = item.video;
  if (video === undefined) return emptyFacts;
  const unplayable = unplayableText(video);
  return {
    ...emptyFacts,
    thumbnailUrl: video.thumbnailUrl,
    narrow: isNarrowVideo(video),
    state: watchState(video),
    ratio: watchedRatio(video),
    unplayable: unplayable === null ? null : String(unplayable),
  };
}

/**
 * orderedItemTitle は項目の名前である。動画は題名、グループは名前。応答に中身が無い項目
 * （契約上は起きない）は空にする。
 */
export function orderedItemTitle(item: OrderedItem): string {
  if (item.kind === "group") return item.group?.name ?? "";
  return item.video?.title ?? "";
}

/** orderedItemDurationMs は項目の長さである。動画の長さか、グループの合計。 */
export function orderedItemDurationMs(item: OrderedItem): number | undefined {
  if (item.kind === "group") return item.group?.durationMs;
  return item.video?.durationMs;
}

/** ItemThumbnailCell は行のサムネイルの列である。長さと進み具合を重ねる。 */
export function ItemThumbnailCell({
  item,
  facts,
}: {
  item: OrderedItem;
  facts: RowFacts;
}) {
  const duration = formatDuration(orderedItemDurationMs(item));
  return (
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
  );
}

/** ItemTitleExtras は題名の下の行（グループの本数、再生できない理由）である。 */
export function ItemTitleExtras({ facts }: { facts: RowFacts }) {
  return (
    <>
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
    </>
  );
}

/**
 * ItemWatchedAndDurationCells は視聴済み（`sm` から）と長さ（`sm` から）の 2 列である。
 * 狭い幅ではサムネイルの長さだけを出し、題名の幅を残す。
 */
export function ItemWatchedAndDurationCells({
  item,
  facts,
}: {
  item: OrderedItem;
  facts: RowFacts;
}) {
  return (
    <>
      <TableCell className="hidden w-16 pr-4 text-right text-xs text-muted-foreground tabular-nums sm:table-cell">
        {facts.state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
        {facts.groupProgress}
      </TableCell>
      <TableCell className="hidden w-0 pr-2 text-right tabular-nums sm:table-cell">
        {formatDuration(orderedItemDurationMs(item))}
      </TableCell>
    </>
  );
}
