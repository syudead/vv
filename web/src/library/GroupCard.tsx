import { Check, Folder } from "lucide-react";
import { memo, type MouseEvent, type ReactNode } from "react";
import { Link } from "react-router";

import type { LibraryGroup } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { useAudience } from "../auth/audience";
import { formatRelative, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration } from "../lib/format";
import AddToGroupButton from "../groups/AddToGroupButton";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
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
import { groupPagePath } from "../groups/groupPaths";
import { type CardDragBinding, dragRowProps, RowDropMarks } from "../videoList/cardDrag";
import FolderArt from "../videoList/FolderArt";

export interface GroupCardProps {
  group: LibraryGroup;
  backTo: string;
  /** Default to the group page without a trail; group-page callers extend their current trail. */
  to?: string;
  /** True only when every member is selected. */
  selected: boolean;
  selectionMode: boolean;
  /** Toggle all members and retain group identity; omit for guests to hide selection. */
  onSelect?: (group: LibraryGroup, selected: boolean) => void;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
  tagsRow?: (group: LibraryGroup) => ReactNode;
  /** Newly created groups may edit their name outside the navigation link. */
  nameField?: ReactNode;
  drag?: CardDragBinding;
}

export type CollectionValues = Pick<
  LibraryGroup,
  "videoCount" | "watchedCount" | "watchState" | "durationMs"
>;

/** Hide watched state for guests; grid cards use watched counts only for progress and accessible names. */
export function useCollectionFacts(values: CollectionValues) {
  const owner = useAudience() === "owner";
  const watchedCount = owner ? (values.watchedCount ?? 0) : 0;
  const state = owner ? (values.watchState ?? "unwatched") : "unwatched";
  const ratio =
    state === "inProgress" && values.videoCount > 0
      ? Math.min(watchedCount / values.videoCount, 1)
      : null;
  return {
    watchedCount,
    state,
    ratio,
    duration: formatDuration(values.durationMs),
    countText: groupCountText(values.videoCount),
  };
}

function useGroupFacts(group: LibraryGroup) {
  const facts = useCollectionFacts(group);
  return {
    ...facts,
    label: groupLinkLabel(group.name, group.videoCount, facts.watchedCount),
  };
}

export function groupCountText(videoCount: number): UiText {
  return t.library.group.count(videoCount);
}

export function groupLinkLabel(
  name: string,
  videoCount: number,
  watchedCount: number,
): UiText {
  return t.library.group.label(name, videoCount, watchedCount);
}

/** Favorite the group itself, never its members. */
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
      onToggle={() => updateFavorites([], [group.id], !favorite)}
      variant={variant}
    />
  );
}

function GroupAddToPlaylist({
  group,
  variant,
}: {
  group: LibraryGroup;
  variant: "card" | "row";
}) {
  return (
    <AddToPlaylistButton
      variant={variant}
      label={t.library.group.addToPlaylist(group.name)}
      groupIds={[group.id]}
    />
  );
}

function GroupAddToGroup({
  group,
  variant,
}: {
  group: LibraryGroup;
  variant: "card" | "row";
}) {
  return (
    <AddToGroupButton
      variant={variant}
      label={t.library.group.addToGroup(group.name)}
      groupIds={[group.id]}
    />
  );
}

/** Keep count/duration and progress above foreground thumbnails without intercepting preview input. */
export function CollectionThumbnail({
  previews,
  selected = false,
  selectionMode = false,
  activePreviewId,
  previewResetEpoch,
  onPreviewStart,
  ratio,
  face,
}: {
  previews: LibraryGroup["previews"];
  selected?: boolean;
  selectionMode?: boolean;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  ratio: number | null;
  face: ReactNode;
}) {
  return (
    <VideoThumbnail selected={selected}>
      <FolderArt
        previews={previews}
        selectionMode={selectionMode}
        activePreviewId={activePreviewId}
        previewResetEpoch={previewResetEpoch}
        onPreviewStart={onPreviewStart}
      />
      <VideoThumbnailDuration className="pointer-events-none z-20">
        {face}
      </VideoThumbnailDuration>
      {ratio !== null && (
        <VideoThumbnailProgress
          value={Math.round(ratio * 100)}
          aria-label={t.library.group.watchedRatio}
          className="z-20"
        />
      )}
    </VideoThumbnail>
  );
}

export function CollectionName({ name, muted }: { name: string; muted: boolean }) {
  return (
    <h3
      title={name}
      className={cn(
        "line-clamp-2 text-sm font-medium break-all",
        muted ? "text-muted-foreground" : "text-foreground",
      )}
    >
      {name}
    </h3>
  );
}

export const GroupCard = memo(function GroupCard(props: GroupCardProps) {
  const {
    group,
    backTo,
    to = groupPagePath(group.id),
    selected,
    selectionMode,
    onSelect,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart,
    onPreviewReset,
    tagsRow,
    nameField,
  } = props;
  const { state, ratio, duration, countText, label } = useGroupFacts(group);
  const owner = useAudience() === "owner";
  const tagsRowNode = tagsRow?.(group);
  const showTagsRow = tagsRow !== undefined && group.tags.length > 0;

  return (
    <article
      data-group-id={group.id}
      data-group-root={group.folder?.rootId}
      data-group-path={group.folder?.path}
      className={cn(
        "group relative flex min-w-0 flex-col gap-2",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
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
        to={to}
        state={{ from: backTo }}
        aria-label={label}
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(group, !selected);
          }
        }}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <CollectionThumbnail
          previews={group.previews}
          selected={selected}
          selectionMode={selectionMode}
          activePreviewId={activePreviewId}
          previewResetEpoch={previewResetEpoch}
          onPreviewStart={onPreviewStart}
          ratio={ratio}
          face={
            <>
              <span>{countText}</span>
              {duration !== "" && <span>{duration}</span>}
            </>
          }
        />

        {nameField === undefined && (
          <CollectionName name={group.name} muted={state === "watched"} />
        )}
      </Link>
      {nameField}
      {owner && (
        //
        <VideoThumbnailMark className="z-30">
          <GroupAddToGroup group={group} variant="card" />
          <GroupAddToPlaylist group={group} variant="card" />
          <GroupFavorite group={group} variant="card" />
        </VideoThumbnailMark>
      )}
      {showTagsRow && <div className="flex min-w-0 flex-col gap-1">{tagsRowNode}</div>}
    </article>
  );
});

export const GroupRow = memo(function GroupRow(props: GroupCardProps) {
  const {
    group,
    backTo,
    to = groupPagePath(group.id),
    selected,
    selectionMode,
    onSelect,
    nameField,
    drag,
  } = props;
  const { watchedCount, state, ratio, duration, label } = useGroupFacts(group);
  const owner = useAudience() === "owner";
  const cover = group.previews[0];

  return (
    <TableRow
      {...dragRowProps(drag, "group")}
      data-group-id={group.id}
      data-group-root={group.folder?.rootId}
      data-group-path={group.folder?.path}
      data-state={selected ? "selected" : undefined}
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
        {nameField ?? (
          <Link
            to={to}
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
        )}
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <Folder aria-hidden="true" className="size-3 shrink-0" />
          {groupCountText(group.videoCount)}
        </span>
        <RowDropMarks drag={drag} />
      </TableCell>
      {owner && (
        <TableCell className="w-8 px-0">
          <GroupFavorite group={group} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <GroupAddToPlaylist group={group} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <GroupAddToGroup group={group} variant="row" />
        </TableCell>
      )}
      <CollectionValueCells
        state={state}
        watchedCount={watchedCount}
        videoCount={group.videoCount}
        duration={duration}
        sizeBytes={group.sizeBytes}
        addedAt={group.addedAt}
      />
    </TableRow>
  );
});

export function CollectionValueCells({
  state,
  watchedCount,
  videoCount,
  duration,
  sizeBytes,
  addedAt,
}: {
  state: ReturnType<typeof useCollectionFacts>["state"];
  watchedCount: number;
  videoCount: number;
  duration: string;
  sizeBytes: number | undefined;
  addedAt: string | undefined;
}) {
  return (
    <>
      <TableCell className="hidden w-16 pr-4 text-right text-xs text-muted-foreground tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
        {state === "inProgress" && t.library.group.progress(watchedCount, videoCount)}
      </TableCell>
      <TableCell className="w-list-number pr-4 text-right tabular-nums">
        {duration}
      </TableCell>
      <TableCell className="hidden w-list-number pr-4 md:table-cell" />
      <TableCell className="hidden w-list-number-wide pr-4 text-right text-muted-foreground tabular-nums md:table-cell">
        {sizeBytes === undefined ? "" : formatBytes(sizeBytes)}
      </TableCell>
      <TableCell className="hidden w-list-date pr-3 text-right text-muted-foreground tabular-nums lg:table-cell">
        {addedAt === undefined ? "" : formatRelative(addedAt)}
      </TableCell>
    </>
  );
}
