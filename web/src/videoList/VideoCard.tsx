import { AlertTriangle, Check, Folder, Globe } from "lucide-react";
import {
  memo,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  useCallback,
  useId,
  useRef,
} from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { updateFavorites } from "../api/favorites";
import { useAudience } from "../auth/audience";
import { formatRelative, t } from "../i18n";
import { cn } from "../lib/cn";
import {
  formatBytes,
  formatDuration,
  qualityLabel,
  unplayableText,
  watchState,
  isNarrowVideo,
  watchedRatio,
} from "../lib/format";
import AddToGroupButton from "../groups/AddToGroupButton";
import AddToPlaylistButton from "../playlists/AddToPlaylistButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { ScrubBand, type ScrubPreview, useScrubPreview } from "../ui/ScrubPreview";
import { Checkbox } from "../ui/shadcn/checkbox";
import { TableCell, TableRow } from "../ui/shadcn/table";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailMark,
  VideoThumbnailNotice,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import { useCardActions } from "./cardActions";
import { type CardDragBinding, dragRowProps, RowDropMarks } from "./cardDrag";
import { cardLinkState } from "./cardLink";
import { CardMedia, useCardPreview } from "./cardPreview";
import { type PreviewCoordinator, usePreviewChanges } from "./usePreviewCoordination";
import { shownVersionsCount, VersionsMark } from "./VersionsMark";
import { VirtualMark } from "./VirtualMark";

export interface VideoCardProps {
  video: Video;
  /** Return URL for playback. */
  backTo: string;
  /** Defaults to /videos/{id}; group pages supply group query context. */
  to?: string;
  selected: boolean;
  selectionMode: boolean;
  /** Omit in views without selection, such as folders. */
  onSelect?: (id: number, selected: boolean) => void;
  previewCoordinator?: PreviewCoordinator;
  /**
   * Truncate label at the start; title retains the complete location, including root absolute path
   * for top-level searches.
   */
  location?: { label: string; title: string };
  /**
   * Render tags outside the link to avoid nested keyboard controls. Accept a stable callback
   * instead of ReactNode so unrelated parent renders preserve memo(VideoCard).
   */
  tagsRow?: (video: Video) => ReactNode;
  /** Owner library rows supply drag metadata; grid cards use DraggableCard wrappers. */
  drag?: CardDragBinding;
  /**
   * 格子のカードが開く再生画面で再生を始める（`state.autoplay`）。ホームの棚のカードが渡す
   * （docs/design-docs/home.md）。
   */
  autoplay?: boolean;
  /**
   * 格子のカードの右上に、所有者のお気に入りの付け外しだけを置く（「Add to group」と
   * 「Add to playlist」を置かない）。ホームの棚の card-0 のカードが渡す
   * （docs/design-docs/home.md「Home」）。
   */
  favoriteOnly?: boolean;
}

function useCardState(video: Video) {
  return {
    duration: formatDuration(video.durationMs),
    unplayable: unplayableText(video),
    state: watchState(video),
    ratio: watchedRatio(video),
    quality: qualityLabel(video),
  };
}

/** Hide public marks for guests because every visible video is already public. */
function usePublicMark(video: Video): boolean {
  return useAudience() === "owner" && video.public;
}

function VideoFavorite({ video, variant }: { video: Video; variant: "card" | "row" }) {
  const favorite = video.favorite === true;
  return (
    <FavoriteToggle
      favorite={favorite}
      label={t.list.card.favorite(video.title)}
      onToggle={() => updateFavorites([video.id], [], !favorite)}
      variant={variant}
    />
  );
}

function VideoAddToPlaylist({
  video,
  variant,
}: {
  video: Video;
  variant: "card" | "row";
}) {
  return (
    <AddToPlaylistButton
      variant={variant}
      label={t.list.card.addToPlaylist(video.title)}
      videoIds={[video.id]}
    />
  );
}

function VideoAddToGroup({ video, variant }: { video: Video; variant: "card" | "row" }) {
  return (
    <AddToGroupButton
      variant={variant}
      label={t.list.card.addToGroup(video.title)}
      videoIds={[video.id]}
    />
  );
}

/** Until the card is approached only a favorite heart, which shows at rest, is created. */
function CardActions({
  video,
  ready,
  favoriteOnly,
  onPointerEnter,
}: {
  video: Video;
  ready: boolean;
  favoriteOnly: boolean;
  onPointerEnter: () => void;
}) {
  if (!ready && video.favorite !== true) return null;
  const all = ready && !favoriteOnly;
  return (
    // Stop previews on entry and never start them from these actions.
    <VideoThumbnailMark data-preview-checkbox="true" onPointerEnter={onPointerEnter}>
      {all && <VideoAddToGroup video={video} variant="card" />}
      {all && <VideoAddToPlaylist video={video} variant="card" />}
      <VideoFavorite video={video} variant="card" />
    </VideoThumbnailMark>
  );
}

function PublicMark() {
  return (
    <>
      <Globe aria-hidden="true" className="size-3 shrink-0" />
      <span className="sr-only">{t.list.card.public}</span>
    </>
  );
}

/** A virtual video's mark takes the duration's place; a file video's time follows the scrub position. */
function CardDuration({
  video,
  duration,
  versions,
  versionsId,
  virtualId,
  publicMark,
  scrubPositionMs,
}: {
  video: Video;
  duration: string;
  versions: number | null;
  versionsId: string;
  virtualId: string;
  publicMark: boolean;
  scrubPositionMs: number | null;
}) {
  if (versions === null && !publicMark && duration === "" && !video.virtual) return null;
  return (
    <VideoThumbnailDuration>
      {versions !== null && <VersionsMark count={versions} id={versionsId} />}
      {publicMark && <PublicMark />}
      {video.virtual && <VirtualMark id={virtualId} />}
      {duration !== "" && (
        <span>
          {scrubPositionMs === null
            ? duration
            : t.list.card.scrubTime(formatDuration(scrubPositionMs), duration)}
        </span>
      )}
    </VideoThumbnailDuration>
  );
}

/** The link name replaces child text, so the marks' accessible text is referenced as its description. */
function describedBy(
  video: Video,
  versions: number | null,
  versionsId: string,
  virtualId: string,
): string | undefined {
  if (video.virtual) return virtualId;
  return versions === null ? undefined : versionsId;
}

/**
 * Keep selection outside the link and stop previews on hover; show on selection, hover, focus or
 * touch-only devices.
 */
function SelectCheck({
  video,
  selected,
  selectionMode,
  onSelect,
  onPreviewCancel,
}: Pick<VideoCardProps, "video" | "selected" | "selectionMode"> & {
  onSelect: (id: number, selected: boolean) => void;
  onPreviewCancel: () => void;
}) {
  return (
    <VideoThumbnailMark
      corner="top-start"
      data-preview-checkbox="true"
      onPointerEnter={onPreviewCancel}
      className={cn(
        "p-1 transition-opacity duration-150",
        selectionMode || selected
          ? "opacity-100"
          : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
      )}
    >
      <Checkbox
        checked={selected}
        onCheckedChange={(next) => onSelect(video.id, next === true)}
        aria-label={t.list.card.select(video.title)}
        onClick={(event: MouseEvent) => event.stopPropagation()}
      />
    </VideoThumbnailMark>
  );
}

/**
 * Pause/resume loop previews across the scrub band; exiting the card delegates release without
 * resuming. Resets, other active previews and release abort scrub requests.
 */
function useCardScrub({
  video,
  selectionMode,
  previewCoordinator,
  preview,
}: {
  video: Video;
  selectionMode: boolean;
  previewCoordinator?: PreviewCoordinator;
  preview: ReturnType<typeof useCardPreview>;
}) {
  const { releasePreview, suspendPreview, resumePreview } = preview;
  const scrub = useScrubPreview({
    video,
    selectionMode,
    onSuspend: suspendPreview,
    onResume: resumePreview,
  });
  const { leaveCard } = scrub;
  const articleRef = useRef<HTMLElement | null>(null);
  const { cardRef } = scrub;
  const setArticle = useCallback(
    (element: HTMLElement | null) => {
      articleRef.current = element;
      cardRef(element);
    },
    [cardRef],
  );

  const bandLeave = scrub.bandHandlers.onPointerLeave;
  const onBandLeave = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const next = event.relatedTarget;
      const card = articleRef.current;
      if (card !== null && !(next instanceof Node && card.contains(next))) {
        leaveCard();
        return;
      }
      bandLeave(event);
    },
    [bandLeave, leaveCard],
  );

  const release = useCallback(() => {
    releasePreview();
    leaveCard();
  }, [leaveCard, releasePreview]);

  usePreviewChanges(previewCoordinator, (current, previous) => {
    const otherStarted =
      previous.activeId !== current.activeId &&
      current.activeId !== null &&
      current.activeId !== video.id;
    if (previous.epoch !== current.epoch || otherStarted) leaveCard();
  });

  const band: ScrubPreview = {
    ...scrub,
    bandHandlers: { ...scrub.bandHandlers, onPointerLeave: onBandLeave },
  };
  return { scrub: band, release, setArticle };
}

function VideoCard(props: VideoCardProps) {
  const {
    video,
    backTo,
    to,
    selected,
    selectionMode,
    onSelect,
    previewCoordinator,
    location,
    tagsRow,
    autoplay,
    favoriteOnly = false,
  } = props;
  const { duration, unplayable: rawUnplayable, state, ratio } = useCardState(video);
  // Keep bottom padding identical with and without tags to avoid inconsistent card spacing.
  const tagsRowNode = tagsRow?.(video);
  const publicMark = usePublicMark(video);
  const versions = shownVersionsCount(video);
  const versionsId = useId();
  const virtualId = useId();
  const owner = useAudience() === "owner";
  const showTagsRow = tagsRow !== undefined && video.tags.length > 0;
  const preview = useCardPreview({ video, selectionMode, previewCoordinator });
  const { startPreview } = preview;
  const { scrub, release, setArticle } = useCardScrub({
    video,
    selectionMode,
    previewCoordinator,
    preview,
  });
  //
  const scrubPosition = scrub.position;
  const actions = useCardActions();
  const { approach } = actions;
  const enter = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      approach();
      startPreview(event);
    },
    [approach, startPreview],
  );

  return (
    <article
      ref={setArticle}
      data-video-id={video.id}
      onPointerEnter={enter}
      onPointerLeave={release}
      onFocus={approach}
      className={cn(
        "group relative flex min-w-0 flex-col gap-2",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
        <SelectCheck
          video={video}
          selected={selected}
          selectionMode={selectionMode}
          onSelect={onSelect}
          onPreviewCancel={release}
        />
      )}
      {/* Stretch the link to align tags at the card bottom and keep intervening whitespace clickable. */}
      <Link
        to={to ?? `/videos/${String(video.id)}`}
        state={cardLinkState(backTo, autoplay)}
        aria-label={
          location === undefined
            ? video.title
            : t.list.card.withLocation(video.title, location.label)
        }
        aria-describedby={describedBy(video, versions, versionsId, virtualId)}
        onClick={(event) => {
          previewCoordinator?.reset();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(video.id, !selected);
          }
        }}
        className="flex min-w-0 flex-1 flex-col gap-2 rounded-md"
      >
        <VideoThumbnail selected={selected}>
          <CardMedia video={video} preview={preview} scrubFrame={scrub.frame} />

          <CardDuration
            video={video}
            duration={duration}
            versions={versions}
            versionsId={versionsId}
            virtualId={virtualId}
            publicMark={publicMark}
            scrubPositionMs={scrubPosition?.positionMs ?? null}
          />

          {ratio !== null && (
            <VideoThumbnailProgress
              value={Math.round(ratio * 100)}
              aria-label={t.list.card.watchedRatio}
              // Hide only visually while scrubbing; retain value and accessible state.
              className={cn(scrubPosition !== null && "opacity-0")}
            />
          )}

          {scrubPosition !== null && (
            <span
              aria-hidden="true"
              data-scrub-bar=""
              className="pointer-events-none absolute inset-x-0 bottom-0 h-1 bg-overlay"
            >
              <span
                className="block h-full bg-foreground"
                style={{ width: `${String(scrubPosition.ratio * 100)}%` }}
              />
            </span>
          )}

          {/* Layer the band above time and progress but below warnings and selection. */}
          <ScrubBand scrub={scrub} />

          {rawUnplayable !== null && (
            <VideoThumbnailNotice>
              <AlertTriangle aria-hidden="true" />
              <span>{rawUnplayable}</span>
            </VideoThumbnailNotice>
          )}
        </VideoThumbnail>

        <div className="flex min-w-0 flex-col gap-1">
          <h3
            title={video.title}
            className={cn(
              "line-clamp-2 text-sm font-medium break-all",
              state === "watched" ? "text-muted-foreground" : "text-foreground",
            )}
          >
            {video.title}
          </h3>
          {location !== undefined && (
            <p className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
              <Folder aria-hidden="true" className="size-3 shrink-0" />
              {/* Preserve path suffixes visually; title retains the full path. */}
              <span
                dir="rtl"
                title={location.title}
                className="min-w-0 truncate text-left"
              >
                <bdi dir="ltr">{location.label}</bdi>
              </span>
            </p>
          )}
        </div>
      </Link>
      {/* Keep mark actions outside the link in checkbox, link, marks, tags DOM order. */}
      {owner && (
        <CardActions
          video={video}
          ready={actions.ready}
          favoriteOnly={favoriteOnly}
          onPointerEnter={release}
        />
      )}
      {showTagsRow && <div className="flex min-w-0 flex-col gap-1">{tagsRowNode}</div>}
    </article>
  );
}

export default memo(VideoCard);

export const VideoRow = memo(function VideoRow(props: VideoCardProps) {
  const { video, backTo, to, selected, selectionMode, onSelect, drag } = props;
  const { duration, unplayable, state, ratio, quality } = useCardState(video);
  const publicMark = usePublicMark(video);
  const versions = shownVersionsCount(video);
  const owner = useAudience() === "owner";

  return (
    <TableRow
      {...dragRowProps(drag, "group")}
      data-video-id={video.id}
      data-state={selected ? "selected" : undefined}
    >
      {/* Omit the entire selection column for audiences without selection. */}
      {onSelect !== undefined && (
        <TableCell className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(video.id, next === true)}
            aria-label={t.list.card.select(video.title)}
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
          {video.thumbnailUrl !== undefined && isNarrowVideo(video) && (
            <ThumbnailBackdrop src={video.thumbnailUrl} />
          )}
          {video.thumbnailUrl !== undefined && (
            <VideoThumbnailImage src={video.thumbnailUrl} />
          )}
          {ratio !== null && (
            <VideoThumbnailProgress
              value={Math.round(ratio * 100)}
              aria-label={t.list.card.watchedRatio}
            />
          )}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="min-w-0 pr-4 whitespace-normal">
        <Link
          to={to ?? `/videos/${String(video.id)}`}
          state={{ from: backTo }}
          onClick={(event) => {
            if (selectionMode) {
              event.preventDefault();
              onSelect?.(video.id, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 rounded-sm text-sm font-medium break-all hover:text-primary",
            state === "watched" ? "text-muted-foreground" : "text-foreground",
          )}
        >
          {video.title}
        </Link>
        {unplayable !== null && (
          <span className="mt-0.5 flex items-center gap-1 text-xs text-warning">
            <AlertTriangle aria-hidden="true" className="size-3" />
            {unplayable}
          </span>
        )}
        <RowDropMarks drag={drag} />
      </TableCell>
      {owner && (
        <TableCell className="w-8 px-0">
          <VideoFavorite video={video} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <VideoAddToPlaylist video={video} variant="row" />
        </TableCell>
      )}
      {owner && (
        <TableCell className="hidden w-8 px-0 sm:table-cell">
          <VideoAddToGroup video={video} variant="row" />
        </TableCell>
      )}
      <TableCell className="hidden w-16 pr-4 text-right sm:table-cell">
        {state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
      </TableCell>
      <TableCell className="w-list-number pr-4 text-right tabular-nums">
        <span className="inline-flex items-center justify-end gap-1.5">
          {versions !== null && <VersionsMark count={versions} />}
          {publicMark && <PublicMark />}
          {video.virtual && <VirtualMark />}
          {duration}
        </span>
      </TableCell>
      <TableCell className="hidden w-list-number pr-4 text-right font-semibold uppercase md:table-cell">
        {!video.virtual && quality}
      </TableCell>
      <TableCell className="hidden w-list-number-wide pr-4 text-right text-muted-foreground tabular-nums md:table-cell">
        {!video.virtual && formatBytes(video.sizeBytes)}
      </TableCell>
      <TableCell className="hidden w-list-date pr-3 text-right text-muted-foreground tabular-nums lg:table-cell">
        {formatRelative(video.addedAt)}
      </TableCell>
    </TableRow>
  );
});
