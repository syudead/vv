import { Check, Folder } from "lucide-react";
import { memo, type MouseEvent, type ReactNode } from "react";
import { Link } from "react-router";

import type { LibraryGroup } from "../api/client";
import { useAudience } from "../auth/audience";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration, formatRelative } from "../lib/format";
import Checkbox from "../ui/Checkbox";
import FolderArt from "../videoList/FolderArt";

/**
 * GroupCardProps はライブラリのグループのカードと行の props である
 * （specs/017-folder-groups/ui-design.md「Group card」）。グループのカードはフォルダ画面には
 * 出ないので `web/src/library/` に置く（plan.md の Structural Decisions 13）。
 */
export interface GroupCardProps {
  group: LibraryGroup;
  /** 遷移元の一覧 URL。再生画面の「戻る」がここへ帰る。 */
  backTo: string;
  /** 全メンバーが選択に入っているときだけ true（ui-design.md「Pressing and selection」）。 */
  selected: boolean;
  selectionMode: boolean;
  /**
   * 全メンバー（`videoIds`）を選択に入れる・外す。選択を持たない見る人（ゲスト）では
   * 省き、チェックを描かない。
   */
  onSelect?: (videoIds: readonly number[], selected: boolean) => void;
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

/** groupCountText はカードの本数の文字（「12 本」）である。 */
export function groupCountText(videoCount: number): string {
  return `${String(videoCount)} 本`;
}

/** groupLinkLabel はカードのリンクの読み上げ名である（ui-design.md「Accessibility」）。 */
export function groupLinkLabel(
  name: string,
  videoCount: number,
  watchedCount: number,
): string {
  const base = `${name}、${String(videoCount)}本のグループ`;
  return watchedCount >= 1 ? `${base}、${String(watchedCount)}本を視聴済み` : base;
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
  const tagsRowNode = tagsRow?.(group);
  const showTagsRow = tagsRow !== undefined && group.tags.length > 0;

  return (
    <article
      data-group-root={group.folder.rootId}
      data-group-path={group.folder.path}
      className={cn(
        "group relative flex flex-col overflow-hidden rounded-lg bg-surface shadow-card transition-[box-shadow,transform] duration-200 ease-out-quart",
        "has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-link",
        "hover:-translate-y-0.5",
        "motion-reduce:transition-none motion-reduce:hover:translate-y-0",
        "hover:shadow-card-hover",
        selected && "ring-2 ring-accent",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
        <div
          className={cn(
            "absolute top-2 left-2 z-30 transition-opacity duration-150",
            selectionMode || selected
              ? "opacity-100"
              : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
          )}
        >
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(group.videoIds, next)}
            label={`「${group.name}」のグループを選択`}
            onClick={(event: MouseEvent) => event.stopPropagation()}
          />
        </div>
      )}

      <Link
        to={groupPath(group)}
        state={{ from: backTo }}
        aria-label={label}
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(group.videoIds, !selected);
          }
        }}
        className="flex min-w-0 flex-col outline-none"
      >
        <div className="relative aspect-video w-full">
          <FolderArt
            previews={group.previews}
            selectionMode={selectionMode}
            activePreviewId={activePreviewId}
            previewResetEpoch={previewResetEpoch}
            onPreviewStart={onPreviewStart}
          />

          {/* 本数と長さは、フォルダの背板の右下に、動画のカードの長さと同じ面で重ねる。
              前に出たサムネイルより上に置き、絵柄の下見の操作を妨げない。 */}
          <span className="pointer-events-none absolute right-5 bottom-4 z-20 flex items-center gap-1.5 rounded-sm bg-navbar/85 px-1.5 py-0.5 text-[11px] font-medium text-fg tabular-nums backdrop-blur-sm">
            <span>{countText}</span>
            {duration !== "" && <span>{duration}</span>}
          </span>

          {ratio !== null && (
            <span
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(ratio * 100)}
              aria-label="視聴済みの本数の割合"
              className="absolute inset-x-0 bottom-0 z-20 h-[5px] bg-fg-subtle/50"
            >
              <span
                className="block h-full bg-accent"
                style={{ width: `${String(Math.round(ratio * 100))}%` }}
              />
            </span>
          )}
        </div>

        <div
          className={cn("flex min-w-0 flex-col gap-1 px-3 pt-2", !showTagsRow && "pb-3")}
        >
          <h3
            title={group.name}
            className={cn(
              "line-clamp-2 text-sm leading-5 font-medium break-all",
              state === "watched" ? "text-fg-muted" : "text-fg",
            )}
          >
            {group.name}
          </h3>
        </div>
      </Link>
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
  // リスト表示は今回変えない。サムネイルは先頭のメンバーの1枚（previews の先頭）を使う。
  const cover = group.previews[0];

  return (
    <tr
      data-group-root={group.folder.rootId}
      data-group-path={group.folder.path}
      className={cn(
        "group relative transition-colors hover:bg-hover-wash",
        selected && "bg-accent-soft",
      )}
    >
      {onSelect !== undefined && (
        <td className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(group.videoIds, next)}
            label={`「${group.name}」のグループを選択`}
            className={cn(
              "transition-opacity",
              selectionMode || selected
                ? "opacity-100"
                : "opacity-40 group-hover:opacity-100",
            )}
          />
        </td>
      )}
      <td className="w-32 py-1.5 pr-2">
        <div className="relative aspect-video w-28 overflow-hidden rounded-sm bg-navbar">
          {cover !== undefined && (
            <img
              src={cover.thumbnailUrl}
              alt=""
              loading="lazy"
              decoding="async"
              className="relative h-full w-full object-contain"
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
          to={groupPath(group)}
          state={{ from: backTo }}
          aria-label={label}
          onClick={(event) => {
            if (selectionMode) {
              event.preventDefault();
              onSelect?.(group.videoIds, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 text-sm font-medium break-all hover:text-link",
            state === "watched" ? "text-fg-muted" : "text-fg",
          )}
        >
          {group.name}
        </Link>
        <span className="mt-0.5 flex items-center gap-1 text-xs text-fg-muted tabular-nums">
          <Folder aria-hidden="true" className="size-3 shrink-0" />
          {`${String(group.videoCount)} 本`}
        </span>
      </td>
      <td className="hidden w-16 pr-4 text-right text-xs text-fg-muted tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check className="ml-auto size-4 text-success" aria-label="視聴済み" />
        )}
        {state === "inProgress" &&
          `${String(watchedCount)} / ${String(group.videoCount)}`}
      </td>
      <td className="w-20 pr-4 text-right text-sm text-fg tabular-nums">{duration}</td>
      <td className="hidden w-20 pr-4 md:table-cell" />
      <td className="hidden w-24 pr-4 text-right text-sm text-fg-muted tabular-nums md:table-cell">
        {formatBytes(group.sizeBytes)}
      </td>
      <td className="hidden w-28 pr-3 text-right text-sm text-fg-muted tabular-nums lg:table-cell">
        {formatRelative(group.addedAt)}
      </td>
    </tr>
  );
});
