import { AlertTriangle, Check, Folder, FolderSearch } from "lucide-react";

import type { LibraryGroup, Video } from "../api/client";
import type { LibrarySmartFolder } from "../api/smartFolders";
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
import { shownVersionsCount, VersionsMark } from "./VersionsMark";
import { VirtualMark } from "./VirtualMark";

/** Smart-folder items occur only in playlists but share the collection row shape. */
export interface OrderedItem {
  kind: "video" | "group" | "smartFolder";
  video?: Video;
  group?: LibraryGroup;
  smartFolder?: LibrarySmartFolder;
}

export type OrderedCollection = Pick<
  LibraryGroup,
  "name" | "videoCount" | "watchedCount" | "watchState" | "durationMs" | "previews"
> & {
  /** Absent for smart folders with no videos. */
  openVideoId?: number;
};

export function orderedItemCollection(item: OrderedItem): OrderedCollection | undefined {
  if (item.kind === "group") return item.group;
  if (item.kind === "smartFolder") return item.smartFolder;
  return undefined;
}

interface RowFacts {
  thumbnailUrl: string | undefined;
  narrow: boolean;
  state: "unwatched" | "inProgress" | "watched";
  /** Ratio in [0, 1], or null when not partially watched. */
  ratio: number | null;
  groupProgress: string | null;
  groupCount: string | null;
  smartFolder: boolean;
  empty: boolean;
  unplayable: string | null;
}

const emptyFacts: RowFacts = {
  thumbnailUrl: undefined,
  narrow: false,
  state: "unwatched",
  ratio: null,
  groupProgress: null,
  groupCount: null,
  smartFolder: false,
  empty: false,
  unplayable: null,
};

export function rowFacts(item: OrderedItem): RowFacts {
  const group = orderedItemCollection(item);
  if (group !== undefined) {
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
      smartFolder: item.kind === "smartFolder",
      empty: group.videoCount === 0,
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

/** Return empty for a contract-violating item with no content. */
export function orderedItemTitle(item: OrderedItem): string {
  if (item.kind === "video") return item.video?.title ?? "";
  return orderedItemCollection(item)?.name ?? "";
}

export function isVirtualItem(item: OrderedItem): boolean {
  return item.kind === "video" && item.video?.virtual === true;
}

export function orderedItemDurationMs(item: OrderedItem): number | undefined {
  if (item.kind === "video") return item.video?.durationMs;
  return orderedItemCollection(item)?.durationMs;
}

export function ItemThumbnailCell({
  item,
  facts,
}: {
  item: OrderedItem;
  facts: RowFacts;
}) {
  const duration = formatDuration(orderedItemDurationMs(item));
  const virtual = isVirtualItem(item);
  const versions =
    item.kind === "video" && item.video !== undefined
      ? shownVersionsCount(item.video)
      : null;
  return (
    <TableCell className="w-list-thumb-cell">
      <VideoThumbnail className="w-list-thumb rounded-sm">
        {facts.thumbnailUrl !== undefined && facts.narrow && (
          <ThumbnailBackdrop src={facts.thumbnailUrl} />
        )}
        {facts.thumbnailUrl !== undefined && (
          <VideoThumbnailImage src={facts.thumbnailUrl} />
        )}
        {(versions !== null || duration !== "" || virtual) && (
          <VideoThumbnailDuration>
            {versions !== null && <VersionsMark count={versions} />}
            {virtual && <VirtualMark />}
            {duration !== "" && <span>{duration}</span>}
          </VideoThumbnailDuration>
        )}
        {facts.ratio !== null && (
          <VideoThumbnailProgress
            value={Math.round(facts.ratio * 100)}
            aria-label={
              item.kind === "video"
                ? t.list.card.watchedRatio
                : t.library.group.watchedRatio
            }
          />
        )}
      </VideoThumbnail>
    </TableCell>
  );
}

export function CollectionCountIcon({ smartFolder }: { smartFolder: boolean }) {
  const Icon = smartFolder ? FolderSearch : Folder;
  return <Icon aria-hidden="true" className="size-3 shrink-0" />;
}

export function ItemTitleExtras({ facts }: { facts: RowFacts }) {
  return (
    <>
      {facts.groupCount !== null && (
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <CollectionCountIcon smartFolder={facts.smartFolder} />
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
 * Hide watch state and duration columns below sm to preserve title width; the thumbnail retains
 * duration.
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
        {isVirtualItem(item) ? (
          // The thumbnail's mark already carries the accessible text.
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
            <VirtualMark announce={false} />
          </span>
        ) : (
          formatDuration(orderedItemDurationMs(item))
        )}
      </TableCell>
    </>
  );
}
