import { FolderSearch } from "lucide-react";
import { memo, type ReactNode, useState } from "react";
import { Link } from "react-router";

import type { SmartFolderSummary, VideoTag } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { smartFolderKey } from "../api/libraryItems";
import type { LibrarySmartFolder } from "../api/smartFolders";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import {
  CollectionCheck,
  CollectionName,
  CollectionRowCheck,
  CollectionThumbnail,
  CollectionValueCells,
  useCollectionFacts,
} from "../library/GroupCard";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { Skeleton } from "../ui/shadcn/skeleton";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import { RefusingDropCard } from "../videoList/cardDrag";
import { useCollectionLink } from "../videoList/useCollectionLink";
import type { PreviewCoordinator } from "../videoList/usePreviewCoordination";
import { useSmartFolderCardValue } from "./cardValues";
import { smartFolderPagePath, videoInSmartFolderPath } from "./smartFolderPaths";

/**
 * Summaries arrive with list items; SmartFolderCardsProvider loads values only when cards become
 * visible.
 */
export interface SmartFolderCardProps {
  folder: SmartFolderSummary;
  /** Return URL for the opened page. */
  backTo: string;
  selected?: boolean;
  selectionMode?: boolean;
  /** Omitted where smart folders cannot be selected, such as for guests. */
  onSelect?: (folderId: number, selected: boolean) => void;
  previewCoordinator?: PreviewCoordinator;
  tagsRow?: (tags: readonly VideoTag[], folderId: number) => ReactNode;
  /** Inside an owner drag layer, this card neither lifts nor accepts drops. */
  refusesDrop?: boolean;
}

const noValues = { videoCount: 0 };

function useSmartFolderFacts(folder: SmartFolderSummary) {
  const { value, ref } = useSmartFolderCardValue(folder.id);
  const values: LibrarySmartFolder | undefined = value === "gone" ? undefined : value;
  const facts = useCollectionFacts(values ?? noValues);
  const label =
    values === undefined
      ? t.library.smartFolder.labelLoading(folder.name)
      : t.library.smartFolder.label(folder.name, values.videoCount, facts.watchedCount);
  return { gone: value === "gone", values, facts, label, ref };
}

function SmartFolderMark() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span title={t.smartFolders.kind} className="pointer-events-auto flex">
          <FolderSearch aria-hidden="true" className="size-3 shrink-0" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{t.smartFolders.kind}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Apply optimistic favorite state only to the current read object; discard it on every new read
 * identity, even if values match. Otherwise a later change from another tab can remain hidden.
 */
function SmartFolderFavorite({
  folder,
  read,
  variant,
}: {
  folder: SmartFolderSummary;
  /** A new object on every read; initially the summary, then card values. */
  read: { favorite?: boolean };
  variant: "card" | "row";
}) {
  const [toggled, setToggled] = useState<{ read: object; favorite: boolean }>();
  const favorite =
    toggled !== undefined && toggled.read === read
      ? toggled.favorite
      : read.favorite === true;
  return (
    <FavoriteToggle
      favorite={favorite}
      label={t.library.smartFolder.favorite(folder.name)}
      onToggle={async () => {
        await updateFavorites([], [], !favorite, { smartFolderIds: [folder.id] });
        setToggled({ read, favorite: !favorite });
      }}
      variant={variant}
    />
  );
}

function SmartFolderAddToPlaylist({
  folder,
  variant,
}: {
  folder: SmartFolderSummary;
  variant: "card" | "row";
}) {
  return (
    <AddToPlaylistButton
      variant={variant}
      label={t.library.smartFolder.addToPlaylist(folder.name)}
      smartFolderIds={[folder.id]}
    />
  );
}

/** Smart folders cannot be dragged or added to groups; omitted value responses hide the card. */
export const SmartFolderCard = memo(function SmartFolderCard(
  props: SmartFolderCardProps,
) {
  const {
    folder,
    backTo,
    tagsRow,
    refusesDrop = false,
    selected = false,
    selectionMode = false,
    onSelect,
  } = props;
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
  const { linkProps, onFrontChange } = useCollectionLink({
    to: smartFolderPagePath(folder.id),
    backTo,
    name: folder.name,
    label,
    selectionMode,
    videoPath: (id) => videoInSmartFolderPath(id, folder.id),
  });
  if (gone) return null;
  const empty = values?.videoCount === 0;
  const tags = values?.tags ?? [];

  const card = (
    <article
      ref={ref}
      data-smart-folder-id={folder.id}
      className={cn(
        "group relative flex min-w-0 flex-col gap-2",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
        <CollectionCheck
          selected={selected}
          selectionMode={selectionMode}
          label={t.library.smartFolder.select(folder.name)}
          onSelect={(next) => onSelect(folder.id, next)}
        />
      )}
      <Link
        {...linkProps}
        onClick={(event) => {
          props.previewCoordinator?.reset();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(folder.id, !selected);
          }
        }}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <CollectionThumbnail
          previews={values?.previews ?? []}
          totalCount={values?.videoCount}
          onFrontChange={onFrontChange}
          selected={selected}
          selectionMode={selectionMode}
          previewCoordinator={props.previewCoordinator}
          ratio={facts.ratio}
          face={
            <>
              <SmartFolderMark />
              {values === undefined ? (
                <Skeleton aria-hidden="true" className="h-3 w-16" />
              ) : (
                <>
                  <span>{facts.countText}</span>
                  {facts.duration !== "" && <span>{facts.duration}</span>}
                </>
              )}
            </>
          }
        />
        <CollectionName name={folder.name} muted={facts.state === "watched" || empty} />
      </Link>
      {tagsRow !== undefined && tags.length > 0 && (
        <div className="flex min-w-0 flex-col gap-1">{tagsRow(tags, folder.id)}</div>
      )}
    </article>
  );
  return refusesDrop ? (
    <RefusingDropCard id={smartFolderKey(folder.id)}>{card}</RefusingDropCard>
  ) : (
    card
  );
});

export const SmartFolderRow = memo(function SmartFolderRow({
  folder,
  backTo,
  owner,
  selected = false,
  selectionMode = false,
  onSelect,
}: SmartFolderCardProps & { owner: boolean }) {
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
  if (gone) return null;
  const cover = values?.previews[0];
  const empty = values?.videoCount === 0;
  return (
    <TableRow
      ref={ref}
      data-smart-folder-id={folder.id}
      data-state={selected ? "selected" : undefined}
    >
      {onSelect !== undefined ? (
        <CollectionRowCheck
          selected={selected}
          selectionMode={selectionMode}
          label={t.library.smartFolder.select(folder.name)}
          onSelect={(next) => onSelect(folder.id, next)}
        />
      ) : (
        owner && <TableCell className="w-10 pl-3" />
      )}
      <TableCell className="w-list-thumb-cell">
        <VideoThumbnail className="w-list-thumb rounded-sm">
          {cover !== undefined && <VideoThumbnailImage src={cover.thumbnailUrl} />}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="min-w-0 pr-4 whitespace-normal">
        <Link
          to={smartFolderPagePath(folder.id)}
          state={{ from: backTo }}
          aria-label={label}
          onClick={(event) => {
            if (selectionMode && onSelect !== undefined) {
              event.preventDefault();
              onSelect(folder.id, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 rounded-sm text-sm font-medium break-all hover:text-primary",
            facts.state === "watched" || empty
              ? "text-muted-foreground"
              : "text-foreground",
          )}
        >
          {folder.name}
        </Link>
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <SmartFolderMark />
          {values === undefined ? (
            <Skeleton aria-hidden="true" className="h-3 w-16" />
          ) : (
            facts.countText
          )}
        </span>
      </TableCell>
      {owner && (
        <TableCell className="w-8 px-0">
          <SmartFolderFavorite folder={folder} read={values ?? folder} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <SmartFolderAddToPlaylist folder={folder} variant="row" />
        </TableCell>
      )}
      {owner && <TableCell className="hidden w-8 px-0 sm:table-cell" />}
      <CollectionValueCells
        state={facts.state}
        watchedCount={facts.watchedCount}
        videoCount={values?.videoCount ?? 0}
        duration={values === undefined ? "" : facts.duration}
        sizeBytes={values?.sizeBytes}
        addedAt={values?.addedAt}
      />
    </TableRow>
  );
});
