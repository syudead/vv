import { Check, Folder } from "lucide-react";
import { memo, type MouseEvent, type ReactNode } from "react";
import { Link } from "react-router";

import type { LibraryGroup } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { groupRef } from "../api/libraryItems";
import { useAudience } from "../auth/audience";
import { formatRelative, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration } from "../lib/format";
import FavoriteToggle from "../ui/FavoriteToggle";
import { Checkbox } from "../ui/shadcn/checkbox";
import { TableCell, TableRow } from "../ui/shadcn/table";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailMark,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import {
  cardFrameClass,
  cardLinkClass,
  cardThumbnailClass,
} from "../ui/patterns/card-grid";
import FolderArt from "../videoList/FolderArt";

/**
 * GroupCardProps はライブラリのグループのカードと行の props である
 * （specs/017-folder-groups/ui-design.md「Group card」）。グループのカードはフォルダ画面には
 * 出ないので `web/src/library/` に置く。
 */
export interface GroupCardProps {
  group: LibraryGroup;
  /** 遷移元の一覧 URL。再生画面の「戻る」がここへ帰る。 */
  backTo: string;
  /** 全メンバーが選択に入っているときだけ true（ui-design.md「Pressing and selection」）。 */
  selected: boolean;
  selectionMode: boolean;
  /**
   * 全メンバー（`videoIds`）を選択に入れる・外す。呼び出し元はグループとしても覚える
   * （specs/035-favorites/research.md R-7）。選択を持たない見る人（ゲスト）では
   * 省き、チェックを描かない。
   */
  onSelect?: (group: LibraryGroup, selected: boolean) => void;
  /** 一覧のホバープレビューの調整（usePreviewCoordination）。格子のフォルダの絵柄が加わる。 */
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
  /** 題名の下のタグの行（格子表示だけ）。VideoCard の tagsRow と同じく関数で受ける。 */
  tagsRow?: (group: LibraryGroup) => ReactNode;
}

/**
 * useGroupFacts は見る人に見せるグループの視聴の値である。ゲストでは視聴状態を出さない
 * （ui-design.md「Screen boundary」の「ゲスト」）。格子のカードは見終えた本数を数字では
 * 出さず、見ている途中の帯の長さと読み上げ名にだけ使う。
 */
function useGroupFacts(group: LibraryGroup) {
  const owner = useAudience() === "owner";
  const watchedCount = owner ? (group.watchedCount ?? 0) : 0;
  const state = owner ? (group.watchState ?? "unwatched") : "unwatched";
  const ratio =
    state === "inProgress" && group.videoCount > 0
      ? Math.min(watchedCount / group.videoCount, 1)
      : null;
  return {
    watchedCount,
    state,
    ratio,
    duration: formatDuration(group.durationMs),
    countText: groupCountText(group.videoCount),
    label: groupLinkLabel(group.name, group.videoCount, watchedCount),
  };
}

/** groupCountText はカードの本数の文字（「12 videos」）である。 */
export function groupCountText(videoCount: number): UiText {
  return t.library.group.count(videoCount);
}

/** groupLinkLabel はカードのリンクの読み上げ名である（ui-design.md「Accessibility」）。 */
export function groupLinkLabel(
  name: string,
  videoCount: number,
  watchedCount: number,
): UiText {
  return t.library.group.label(name, videoCount, watchedCount);
}

/**
 * GroupFavorite はグループのカード・行のお気に入りの付け外しである（所有者だけ。
 * specs/035-favorites/ui-design.md「Card」）。グループのフォルダを送り、メンバーには付けない。
 */
function GroupFavorite({
  group,
  variant,
}: {
  group: LibraryGroup;
  variant: "card" | "row";
}) {
  const favorite = group.favorite === true;
  return (
    <FavoriteToggle
      favorite={favorite}
      label={t.library.group.favorite(group.name)}
      onToggle={() => updateFavorites([], [groupRef(group)], !favorite)}
      variant={variant}
    />
  );
}

function groupPath(group: LibraryGroup): string {
  return `/videos/${String(group.openVideoId)}`;
}

/**
 * GroupCard はライブラリの格子でグループを1枚で出すカードである。動画のカード
 * （VideoCard）と同じ箱・同じ大きさで、サムネイルの枠にはフォルダ画面のフォルダカードと
 * 同じフォルダの絵柄（FolderArt）を描き、右下の面に本数と合計の長さを重ねる。
 * カード全体が開くメンバーの再生画面への1つのリンクである（要件 16・17・23）。
 */
export const GroupCard = memo(function GroupCard(props: GroupCardProps) {
  const {
    group,
    backTo,
    selected,
    selectionMode,
    onSelect,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart,
    onPreviewReset,
    tagsRow,
  } = props;
  const { state, ratio, duration, countText, label } = useGroupFacts(group);
  const owner = useAudience() === "owner";
  const tagsRowNode = tagsRow?.(group);
  const showTagsRow = tagsRow !== undefined && group.tags.length > 0;

  return (
    <article
      data-group-root={group.folder.rootId}
      data-group-path={group.folder.path}
      className={cn(cardFrameClass(selected), selectionMode && "select-none")}
    >
      {onSelect !== undefined && (
        // 前に出たサムネイルより上に置く（z-30）。
        <VideoThumbnailMark
          corner="top-start"
          className={cn(
            "z-30 p-1 transition-opacity duration-150",
            selectionMode || selected
              ? "opacity-100"
              : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
          )}
        >
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(group, next === true)}
            aria-label={t.library.group.select(group.name)}
            onClick={(event: MouseEvent) => event.stopPropagation()}
          />
        </VideoThumbnailMark>
      )}

      <Link
        to={groupPath(group)}
        state={{ from: backTo }}
        aria-label={label}
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(group, !selected);
          }
        }}
        className={cardLinkClass}
      >
        {/* フォルダの絵柄は背板ごとカードの地に描くので、枠の地は塗らない。 */}
        <VideoThumbnail className={cn(cardThumbnailClass, "bg-transparent")}>
          <FolderArt
            previews={group.previews}
            selectionMode={selectionMode}
            activePreviewId={activePreviewId}
            previewResetEpoch={previewResetEpoch}
            onPreviewStart={onPreviewStart}
          />

          {/* 本数と長さは、フォルダの背板の右下に、動画のカードの長さと同じ面で重ねる。
              前に出たサムネイルより上に置き、絵柄の下見の操作を妨げない。 */}
          <VideoThumbnailDuration className="pointer-events-none right-5 bottom-4 z-20">
            <span>{countText}</span>
            {duration !== "" && <span>{duration}</span>}
          </VideoThumbnailDuration>

          {ratio !== null && (
            <VideoThumbnailProgress
              value={Math.round(ratio * 100)}
              aria-label={t.library.group.watchedRatio}
              className="z-20"
            />
          )}
        </VideoThumbnail>

        <div
          className={cn("flex min-w-0 flex-col gap-1 px-3 pt-2", !showTagsRow && "pb-3")}
        >
          <h3
            title={group.name}
            className={cn(
              "line-clamp-2 text-sm font-semibold break-all sm:text-base",
              state === "watched" ? "text-muted-foreground" : "text-foreground",
            )}
          >
            {group.name}
          </h3>
        </div>
      </Link>
      {owner && (
        // 前に出たサムネイルより上（チェックと同じ z-30）。リンクの外、タグの行の前に置く
        // （ui-design.md「Placement」）。
        <VideoThumbnailMark className="z-30">
          <GroupFavorite group={group} variant="card" />
        </VideoThumbnailMark>
      )}
      {showTagsRow && (
        <div className="flex min-w-0 flex-col gap-1 px-3 pt-1 pb-3">{tagsRowNode}</div>
      )}
    </article>
  );
});

/**
 * GroupRow はリスト表示のグループの行である。動画の行（VideoRow）と同じ列に
 * グループの値を出す（ui-design.md「List view row」）。
 */
export const GroupRow = memo(function GroupRow(props: GroupCardProps) {
  const { group, backTo, selected, selectionMode, onSelect } = props;
  const { watchedCount, state, ratio, duration, label } = useGroupFacts(group);
  const owner = useAudience() === "owner";
  // リスト表示は今回変えない。サムネイルは先頭のメンバーの1枚（previews の先頭）を使う。
  const cover = group.previews[0];

  return (
    <TableRow
      data-group-root={group.folder.rootId}
      data-group-path={group.folder.path}
      data-state={selected ? "selected" : undefined}
      className="group"
    >
      {onSelect !== undefined && (
        <TableCell className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(group, next === true)}
            aria-label={t.library.group.select(group.name)}
            className={cn(
              "transition-opacity",
              selectionMode || selected
                ? "opacity-100"
                : "opacity-50 group-focus-within:opacity-100 group-hover:opacity-100",
            )}
          />
        </TableCell>
      )}
      <TableCell className="w-list-thumb-cell">
        <VideoThumbnail className="w-list-thumb rounded-sm">
          {cover !== undefined && <VideoThumbnailImage src={cover.thumbnailUrl} />}
          {ratio !== null && (
            <VideoThumbnailProgress
              value={Math.round(ratio * 100)}
              aria-label={t.library.group.watchedRatio}
            />
          )}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="min-w-0 pr-4 whitespace-normal">
        <Link
          to={groupPath(group)}
          state={{ from: backTo }}
          aria-label={label}
          onClick={(event) => {
            if (selectionMode) {
              event.preventDefault();
              onSelect?.(group, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 rounded-sm text-sm font-medium break-all hover:text-primary",
            state === "watched" ? "text-muted-foreground" : "text-foreground",
          )}
        >
          {group.name}
        </Link>
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <Folder aria-hidden="true" className="size-3 shrink-0" />
          {groupCountText(group.videoCount)}
        </span>
      </TableCell>
      {owner && (
        <TableCell className="w-8 px-0">
          <GroupFavorite group={group} variant="row" />
        </TableCell>
      )}
      <TableCell className="hidden w-16 pr-4 text-right text-xs text-muted-foreground tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
        {state === "inProgress" &&
          t.library.group.progress(watchedCount, group.videoCount)}
      </TableCell>
      <TableCell className="w-list-number pr-4 text-right tabular-nums">
        {duration}
      </TableCell>
      <TableCell className="hidden w-list-number pr-4 md:table-cell" />
      <TableCell className="hidden w-list-number-wide pr-4 text-right text-muted-foreground tabular-nums md:table-cell">
        {formatBytes(group.sizeBytes)}
      </TableCell>
      <TableCell className="hidden w-list-date pr-3 text-right text-muted-foreground tabular-nums lg:table-cell">
        {formatRelative(group.addedAt)}
      </TableCell>
    </TableRow>
  );
});
