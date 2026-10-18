import { AlertTriangle, Check, ListVideo } from "lucide-react";
import { type ReactNode, useLayoutEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import type { PlaylistItem } from "../api/playlists";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, unplayableText } from "../lib/format";
import {
  CardNumber,
  cardListClass,
  isWideScreen,
  stickyHeadingClass,
  useRelatedScrub,
  VideoThumbnail as RelatedThumbnail,
} from "../player/RelatedVideos";
import { useHoverPreview } from "../player/useHoverPreview";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import { CollectionCountIcon, orderedItemCollection } from "../videoList/ItemRowCells";
import { itemTitle, playlistPath } from "./playlistItems";
import { itemStepPath, panelScrollTop } from "./playlistPlayback";
import type { PlaylistPanelState } from "./usePlaylistContext";

const headingId = "playlist-panel-heading";

/**
 * Scroll only the panel's container to reveal playback; never move the page or aside column.
 * Editing belongs on the playlist page.
 */
export default function PlaylistPanel({
  playlistId,
  panel,
  name,
  position,
  count,
  currentItemId,
  backTo,
  autoplay,
  onRetry,
  scrollToCurrent = true,
}: {
  playlistId: number;
  panel: PlaylistPanelState;
  /** Fall back to the playback response's name until the panel arrives. */
  name: string | undefined;
  /** Hide position information when unknown or zero. */
  position: number;
  count: number;
  currentItemId: number | undefined;
  backTo: string;
  autoplay: boolean;
  onRetry: () => void;
  /** Disable auto-scroll when Up next below the panel owns scrolling to its current member. */
  scrollToCurrent?: boolean;
}) {
  const shownName = panel.kind === "ready" ? panel.playlist.name : name;
  return (
    <section
      aria-labelledby={shownName === undefined ? undefined : headingId}
      data-playlist-panel=""
      // The section owns right padding so wide-screen rows align with the lists below.
      className="flex shrink-0 flex-col"
    >
      <div className={stickyHeadingClass}>
        {shownName === undefined ? (
          <Skeleton aria-hidden="true" className="h-5 w-1/2" />
        ) : (
          <h2
            id={headingId}
            className="flex min-w-0 items-baseline gap-1.5 text-sm font-semibold"
          >
            <ListVideo
              aria-hidden="true"
              className="size-4 shrink-0 self-center text-muted-foreground"
            />
            <Link
              to={playlistPath(playlistId)}
              state={{ from: backTo }}
              title={shownName}
              aria-label={t.player.playlist.line(shownName, 0, count)}
              className="min-w-0 truncate rounded-sm hover:underline"
            >
              {shownName}
            </Link>
          </h2>
        )}
        {shownName !== undefined && position > 0 && (
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {t.player.related.position(position, count)}
          </span>
        )}
      </div>
      {panel.kind === "loading" && (
        <ul
          aria-hidden="true"
          className={cn(
            cardListClass,
            "max-h-playlist-panel overflow-hidden lg:max-h-none",
          )}
        >
          {Array.from({ length: 3 }, (_, index) => (
            <li key={index} className="flex flex-col gap-2">
              <Skeleton className="aspect-video w-full" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-2/3" />
            </li>
          ))}
        </ul>
      )}
      {panel.kind === "failed" && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-sm text-muted-foreground">{t.playlists.detail.loadFailed}</p>
          <Button variant="ghost" size="sm" onClick={onRetry}>
            {t.common.retry}
          </Button>
        </div>
      )}
      {panel.kind === "ready" && panel.playlist.items.length > 0 && (
        <PanelRows
          playlistId={playlistId}
          items={panel.playlist.items}
          currentItemId={currentItemId}
          backTo={backTo}
          autoplay={autoplay}
          scrollToCurrent={scrollToCurrent}
        />
      )}
    </section>
  );
}

function PanelRows({
  playlistId,
  items,
  currentItemId,
  backTo,
  autoplay,
  scrollToCurrent,
}: {
  playlistId: number;
  items: readonly PlaylistItem[];
  currentItemId: number | undefined;
  backTo: string;
  autoplay: boolean;
  scrollToCurrent: boolean;
}) {
  const listRef = useRef<HTMLUListElement | null>(null);
  const currentRef = useRef<HTMLLIElement | null>(null);
  const currentIndex = items.findIndex((item) => item.id === currentItemId);

  // Reveal the active item after mount, playback changes or reordered responses, using the panel on narrow screens and the shared list container on wide screens.
  // Avoid scrollIntoView because it also scrolls the page and aside column.
  useLayoutEffect(() => {
    const list = listRef.current;
    const current = currentRef.current;
    if (!scrollToCurrent || list === null || current === null) return;
    const previous = current.previousElementSibling;
    const top = panelScrollTop(
      previous instanceof HTMLElement
        ? { top: previous.offsetTop, height: previous.offsetHeight }
        : null,
    );
    const scroller = isWideScreen()
      ? list.closest<HTMLElement>("[data-related-scroller]")
      : null;
    if (scroller === null) {
      list.scrollTop = top;
      return;
    }
    // Offset by the sticky heading height to keep the row visible beneath it.
    const heading = list.parentElement?.firstElementChild;
    const covered = heading instanceof HTMLElement ? heading.offsetHeight : 0;
    scroller.scrollTop = Math.max(0, top - covered);
  }, [currentItemId, currentIndex, scrollToCurrent]);

  return (
    <ul
      ref={listRef}
      aria-labelledby={headingId}
      data-playlist-panel-list=""
      // Allow padding for the hover surface and outline to avoid clipping.
      className={cn(
        cardListClass,
        "relative -mx-1.5 max-h-playlist-panel overflow-y-auto overscroll-contain scrollbar-on-hover px-1.5 py-1.5 lg:static lg:max-h-none lg:overflow-visible",
      )}
    >
      {items.map((item, index) =>
        item.id === currentItemId ? (
          <li key={item.id} ref={currentRef} aria-current="true">
            <div className="-m-1.5 flex flex-col gap-2 rounded-lg bg-accent p-1.5 ring-2 ring-primary ring-inset">
              <ItemThumbnail item={item} position={index + 1} />
              <span className="sr-only">{t.player.related.nowPlaying}</span>
              <ItemText item={item} current />
            </div>
          </li>
        ) : (
          <PanelRow
            key={item.id}
            playlistId={playlistId}
            item={item}
            position={index + 1}
            backTo={backTo}
            autoplay={autoplay}
          />
        ),
      )}
    </ul>
  );
}

function PanelRow({
  playlistId,
  item,
  position,
  backTo,
  autoplay,
}: {
  playlistId: number;
  item: PlaylistItem;
  position: number;
  backTo: string;
  autoplay: boolean;
}) {
  const to = itemStepPath(playlistId, item);
  const content = (
    <>
      <ItemThumbnail item={item} position={position} />
      <ItemText item={item} />
    </>
  );
  if (to === null) {
    return (
      <li>
        <div className="flex flex-col gap-2">{content}</div>
      </li>
    );
  }
  const state = { from: backTo, autoplay };
  const label = t.playlists.item.playFrom(itemTitle(item));
  if (item.kind === "video" && item.video !== undefined) {
    return (
      <VideoRow
        video={item.video}
        to={to}
        state={state}
        label={label}
        position={position}
        text={<ItemText item={item} />}
      />
    );
  }
  return (
    <li>
      <Link
        to={to}
        state={state}
        aria-label={label}
        className="-m-1.5 flex flex-col gap-2 rounded-lg p-1.5 transition-colors hover:bg-accent"
      >
        {content}
      </Link>
    </li>
  );
}

function VideoRow({
  video,
  to,
  state,
  label,
  position,
  text,
}: {
  video: Video;
  to: string;
  state: { from: string; autoplay: boolean };
  label: string;
  position: number;
  text: ReactNode;
}) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  return (
    <li>
      <Link
        ref={setLink}
        to={to}
        state={state}
        aria-label={label}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex flex-col gap-2 rounded-lg p-1.5 transition-colors hover:bg-accent"
      >
        <RelatedThumbnail
          video={video}
          preview={preview}
          scrub={scrub}
          marks={<CardNumber position={position} />}
        />
        {text}
      </Link>
    </li>
  );
}

/** Current items, groups and smart folders do not play thumbnail previews. */
function ItemThumbnail({ item, position }: { item: PlaylistItem; position: number }) {
  if (item.kind === "video" && item.video !== undefined) {
    return (
      <RelatedThumbnail video={item.video} marks={<CardNumber position={position} />} />
    );
  }
  const group = orderedItemCollection(item);
  const thumbnailUrl = group?.previews[0]?.thumbnailUrl;
  const duration = formatDuration(group?.durationMs);
  const inProgress = group?.watchState === "inProgress" && group.videoCount > 0;
  const ratio = inProgress
    ? Math.min((group.watchedCount ?? 0) / group.videoCount, 1)
    : 0;
  return (
    <VideoThumbnail className="shrink-0">
      {thumbnailUrl !== undefined && <VideoThumbnailImage src={thumbnailUrl} />}
      {duration !== "" && (
        <VideoThumbnailDuration className="right-1 bottom-1 px-1 py-0 text-xs font-normal">
          {duration}
        </VideoThumbnailDuration>
      )}
      {inProgress && (
        <VideoThumbnailProgress
          value={Math.round(ratio * 100)}
          aria-label={t.library.group.watchedRatio}
        />
      )}
      <CardNumber position={position} />
    </VideoThumbnail>
  );
}

function ItemText({ item, current = false }: { item: PlaylistItem; current?: boolean }) {
  const title = itemTitle(item);
  const group = orderedItemCollection(item);
  const watched =
    group !== undefined
      ? group.watchState === "watched"
      : item.video?.progress?.completed === true;
  const groupProgress =
    group !== undefined && group.watchState === "inProgress"
      ? t.library.group.progress(group.watchedCount ?? 0, group.videoCount)
      : current && group?.videoCount === 0
        ? t.library.group.count(0)
        : null;
  const unplayable = item.video === undefined ? null : unplayableText(item.video);
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex min-w-0 items-start gap-1">
        <span
          className={cn(
            "line-clamp-2 min-w-0 text-sm font-medium wrap-anywhere",
            watched || group?.videoCount === 0
              ? "text-muted-foreground"
              : "text-foreground",
          )}
        >
          {title}
        </span>
        {watched && (
          <>
            <Check
              className="mt-0.5 size-4 shrink-0 text-success"
              strokeWidth={2.5}
              aria-hidden="true"
            />
            <span className="sr-only">{t.player.related.watched}</span>
          </>
        )}
        {groupProgress !== null && (
          <span className="ml-auto shrink-0 pt-0.5 text-xs text-muted-foreground tabular-nums">
            {groupProgress}
          </span>
        )}
      </span>
      {group !== undefined && (
        <span className="flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
          <CollectionCountIcon smartFolder={item.kind === "smartFolder"} />
          {t.library.group.count(group.videoCount)}
        </span>
      )}
      {unplayable !== null && (
        <span className="flex items-center gap-1 text-xs text-warning">
          <AlertTriangle aria-hidden="true" className="size-3 shrink-0" />
          {unplayable}
        </span>
      )}
    </span>
  );
}
