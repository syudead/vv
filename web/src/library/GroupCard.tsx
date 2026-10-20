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
import { groupPagePath, videoInGroupPath } from "../groups/groupPaths";
import { type CardDragBinding, dragRowProps, RowDropMarks } from "../videoList/cardDrag";
import FolderArt from "../videoList/FolderArt";
import type { PreviewCoordinator } from "../videoList/usePreviewCoordination";
import { useCollectionLink } from "../videoList/useCollectionLink";

export interface GroupCardProps<G extends LibraryGroup = LibraryGroup> {
  group: G;
  backTo: string;
  /** Default to the group page without a trail; group-page callers extend their current trail. */
  to?: string;
  /** True only when every member is selected. */
  selected: boolean;
  selectionMode: boolean;
  onSelect?: (group: G, selected: boolean) => void;
  previewCoordinator?: PreviewCoordinator;
  tagsRow?: (group: G) => ReactNode;
  nameField?: ReactNode;
  drag?: CardDragBinding;
  /** 格子のカードが開く再生画面で再生を始める（VideoCard の autoplay と同じ）。 */
  autoplay?: boolean;
  /** 格子のカードの右上に、所有者のお気に入りの付け外しだけを置く（VideoCard と同じ）。 */
  favoriteOnly?: boolean;
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
  totalCount,
  onFrontChange,
  selected = false,
  selectionMode = false,
  previewCoordinator,
  ratio,
  face,
}: {
  previews: LibraryGroup["previews"];
  totalCount?: number;
  onFrontChange?: (preview: LibraryGroup["previews"][number] | null) => void;
  selected?: boolean;
  selectionMode?: boolean;
  previewCoordinator?: PreviewCoordinator;
  ratio: number | null;
  face: ReactNode;
}) {
  return (
    <VideoThumbnail selected={selected}>
      <FolderArt
        previews={previews}
        totalCount={totalCount}
        onFrontChange={onFrontChange}
        selectionMode={selectionMode}
        previewCoordinator={previewCoordinator}
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

interface CollectionCheckProps {
  selected: boolean;
  selectionMode: boolean;
  label: UiText;
  onSelect: (selected: boolean) => void;
}

/** A collection card's check, shown on hover or focus, always where hover is impossible, and during selection. */
export function CollectionCheck({
  selected,
  selectionMode,
  label,
  onSelect,
}: CollectionCheckProps) {
  return (
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
        onCheckedChange={(next) => onSelect(next === true)}
        aria-label={label}
        onClick={(event: MouseEvent) => event.stopPropagation()}
      />
    </VideoThumbnailMark>
  );
}

/** A collection row's check cell, always faintly visible in list view. */
export function CollectionRowCheck({
  selected,
  selectionMode,
  label,
  onSelect,
}: CollectionCheckProps) {
  return (
    <TableCell className="w-10 pl-3">
      <Checkbox
        checked={selected}
        onCheckedChange={(next) => onSelect(next === true)}
        aria-label={label}
        className={cn(
          "transition-opacity",
          selectionMode || selected
            ? "opacity-100"
            : "opacity-50 group-focus-within:opacity-100 group-hover:opacity-100",
        )}
      />
    </TableCell>
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

export const GroupCard = memo(GroupCardFor) as typeof GroupCardFor;

function groupHasTags(group: LibraryGroup): boolean {
  return group.tags.length > 0;
}

function GroupCardFor<G extends LibraryGroup>(props: GroupCardProps<G>) {
  const {
    group,
    backTo,
    to = groupPagePath(group.id),
    selected,
    selectionMode,
    onSelect,
    previewCoordinator,
    tagsRow,
    nameField,
    autoplay,
    favoriteOnly = false,
  } = props;
  const { state, ratio, duration, countText, label } = useGroupFacts(group);
  const { linkProps, onFrontChange } = useCollectionLink({
    to,
    backTo,
    name: group.name,
    label,
    selectionMode,
    autoplay,
    videoPath: (id) => videoInGroupPath(id, group.id),
  });
  const owner = useAudience() === "owner";
  const tagsRowNode = tagsRow?.(group);
  const showTagsRow = tagsRow !== undefined && groupHasTags(group);

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
        <CollectionCheck
          selected={selected}
          selectionMode={selectionMode}
          label={t.library.group.select(group.name)}
          onSelect={(next) => onSelect(group, next)}
        />
      )}

      <Link
        {...linkProps}
        onClick={(event) => {
          previewCoordinator?.reset();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(group, !selected);
          }
        }}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <CollectionThumbnail
          previews={group.previews}
          totalCount={group.videoCount}
          onFrontChange={onFrontChange}
          selected={selected}
          selectionMode={selectionMode}
          previewCoordinator={previewCoordinator}
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
          {!favoriteOnly && <GroupAddToGroup group={group} variant="card" />}
          {!favoriteOnly && <GroupAddToPlaylist group={group} variant="card" />}
          <GroupFavorite group={group} variant="card" />
        </VideoThumbnailMark>
      )}
      {showTagsRow && <div className="flex min-w-0 flex-col gap-1">{tagsRowNode}</div>}
    </article>
  );
}

export const GroupRow = memo(GroupRowFor) as typeof GroupRowFor;

function GroupRowFor<G extends LibraryGroup>(props: GroupCardProps<G>) {
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
        <CollectionRowCheck
          selected={selected}
          selectionMode={selectionMode}
          label={t.library.group.select(group.name)}
          onSelect={(next) => onSelect(group, next)}
        />
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
}

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
