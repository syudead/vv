import { FolderSearch, Play } from "lucide-react";
import { memo, type ReactNode, useState } from "react";
import { Link } from "react-router";

import type { SmartFolderSummary, VideoTag } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { smartFolderKey } from "../api/libraryItems";
import type { LibrarySmartFolder } from "../api/smartFolders";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import {
  CollectionName,
  CollectionValueCells,
  useCollectionFacts,
} from "../library/GroupCard";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { CardGridItem } from "../ui/patterns/card-grid";
import { Skeleton } from "../ui/shadcn/skeleton";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import { RefusingDropCard } from "../videoList/cardDrag";
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
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
  tagsRow?: (tags: readonly VideoTag[]) => ReactNode;
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

/**
 * Smart folders cannot be selected, dragged or added to groups; omitted value responses hide the
 * card.
 */
export const SmartFolderCard = memo(function SmartFolderCard(
  props: SmartFolderCardProps,
) {
  const { folder, backTo, tagsRow, refusesDrop = false } = props;
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
  const owner = useAudience() === "owner";
  if (gone) return null;
  const empty = values?.videoCount === 0;
  const tags = values?.tags ?? [];

  const card = (
    <article
      ref={ref}
      data-smart-folder-id={folder.id}
      className="group relative flex min-w-0 flex-col gap-2 rounded-md border border-border bg-card p-3"
    >
      <div className="flex min-w-0 items-start gap-2">
        <Link
          to={smartFolderPagePath(folder.id)}
          state={{ from: backTo }}
          aria-label={label}
          onClick={() => props.onPreviewReset?.()}
          className="flex min-w-0 flex-1 flex-col gap-1 rounded-sm hover:text-primary"
        >
          <CollectionName name={folder.name} muted={facts.state === "watched" || empty} />
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <SmartFolderMark />
            {values === undefined ? (
              <Skeleton aria-hidden="true" className="h-3 w-16" />
            ) : (
              facts.countText
            )}
            {facts.duration !== "" && <span>{facts.duration}</span>}
          </span>
        </Link>
        {owner && (
          <div className="flex shrink-0">
            <SmartFolderAddToPlaylist folder={folder} variant="row" />
            <SmartFolderFavorite folder={folder} read={values ?? folder} variant="row" />
          </div>
        )}
      </div>
      <SmartFolderPreviews
        folder={folder}
        values={values}
        backTo={backTo}
        onOpen={props.onPreviewReset}
      />
      {tagsRow !== undefined && tags.length > 0 && (
        <div className="flex min-w-0 flex-col gap-1">{tagsRow(tags)}</div>
      )}
    </article>
  );
  return (
    <CardGridItem span={2}>
      {refusesDrop ? (
        <RefusingDropCard id={smartFolderKey(folder.id)}>{card}</RefusingDropCard>
      ) : (
        card
      )}
    </CardGridItem>
  );
});

function SmartFolderPreviews({
  folder,
  values,
  backTo,
  onOpen,
}: {
  folder: SmartFolderSummary;
  values: LibrarySmartFolder | undefined;
  backTo: string;
  onOpen?: () => void;
}) {
  if (values === undefined)
    return (
      <div className="grid grid-cols-2 gap-2" aria-busy="true">
        <Skeleton className="aspect-video w-full" />
        <Skeleton className="aspect-video w-full" />
      </div>
    );
  return (
    <div className="grid grid-cols-2 gap-2">
      {values.previews.slice(0, 2).map((preview, index) => {
        const title = preview.title ?? t.library.smartFolder.previewTitle(index + 1);
        return (
          <Link
            key={preview.videoId}
            to={videoInSmartFolderPath(preview.videoId, folder.id)}
            state={{ from: backTo, autoplay: true }}
            aria-label={t.library.smartFolder.playPreview(title, folder.name)}
            onClick={onOpen}
            className="group/preview flex min-w-0 flex-col gap-1 rounded-md hover:text-primary"
          >
            <VideoThumbnail>
              <VideoThumbnailImage src={preview.thumbnailUrl} />
              <span className="absolute right-2 bottom-2 rounded-full bg-overlay p-1.5 text-foreground">
                <Play aria-hidden="true" className="size-4" />
              </span>
            </VideoThumbnail>
            <span className="truncate text-xs" title={title}>
              {title}
            </span>
          </Link>
        );
      })}
    </div>
  );
}

export const SmartFolderRow = memo(function SmartFolderRow({
  folder,
  backTo,
  owner,
}: SmartFolderCardProps & { owner: boolean }) {
  const { gone, values, facts, label, ref } = useSmartFolderFacts(folder);
  if (gone) return null;
  const cover = values?.previews[0];
  const empty = values?.videoCount === 0;
  return (
    <TableRow ref={ref} data-smart-folder-id={folder.id}>
      {owner && <TableCell className="w-10 pl-3" />}
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
