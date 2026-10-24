import { Check, ImageOff } from "lucide-react";
import {
  type PointerEvent,
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link } from "react-router";

import { isAborted, listVideoGroupMembers, type Video } from "../api/client";
import type { RelatedState, VideoReadContext } from "../api/useVideoDetail";
import type { VideoLinkState } from "./pageDecisions";
import { type HoverPreview, useHoverPreview } from "./useHoverPreview";
import { subscribeTheaterMode, theaterMode } from "./theaterMode";
import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, isNarrowVideo, watchedRatio } from "../lib/format";
import {
  ScrubBand,
  ScrubFrame,
  type ScrubPreview,
  useScrubPreview,
} from "../ui/ScrubPreview";
import { Button } from "../ui/shadcn/button";
import { Separator } from "../ui/shadcn/separator";
import { Skeleton } from "../ui/shadcn/skeleton";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import {
  VideoThumbnail as Thumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailMark,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";
import { VirtualMark } from "../videoList/VirtualMark";

/** Keep progress percentages out of the link name while leaving the progress bar accessible. */
export function videoLinkLabel(video: Video): string {
  const duration = formatDuration(video.durationMs);
  return duration === ""
    ? video.title
    : t.player.related.videoLink(video.title, duration);
}

/** Scrubbing temporarily replaces the thumbnail, duration, and progress with the hovered frame. */
export function VideoThumbnail({
  video,
  className,
  preview,
  scrub,
  marks,
  markId,
  selected,
}: {
  video: Video;
  className?: string;
  /** Id of the virtual mark's accessible text, for the enclosing link's aria-describedby. */
  markId?: string;
  /** The `primary` ring of the one playing item in a list. */
  selected?: boolean;
  /** Position badge supplied by continuous-play and playlist cards. */
  marks?: ReactNode;
  preview?: HoverPreview;
  scrub?: ScrubPreview;
}) {
  const duration = formatDuration(video.durationMs);
  const ratio = watchedRatio(video);
  const scrubPosition = scrub?.position ?? null;
  return (
    <Thumbnail selected={selected} className={cn("shrink-0", className)}>
      {video.thumbnailUrl !== undefined && isNarrowVideo(video) && (
        <ThumbnailBackdrop src={video.thumbnailUrl} />
      )}
      {video.thumbnailUrl !== undefined ? (
        <VideoThumbnailImage
          src={video.thumbnailUrl}
          className={cn(preview?.playing === true && "opacity-0")}
        />
      ) : (
        <div className="flex size-full flex-col items-center justify-center gap-1 text-muted-foreground">
          <ImageOff className="size-5" strokeWidth={1.5} aria-hidden="true" />
          <span className="text-2xs">
            {video.thumbnailState === "failed"
              ? t.list.card.noImage
              : t.list.card.preparing}
          </span>
        </div>
      )}
      {preview?.active === true && preview.previewUrl !== undefined && (
        <video
          ref={preview.videoRef}
          src={preview.previewUrl}
          muted
          loop
          playsInline
          preload="auto"
          controls={false}
          aria-hidden="true"
          tabIndex={-1}
          onPlaying={preview.onPlaying}
          onError={preview.onError}
          className={cn(
            "absolute inset-0 size-full object-contain",
            preview.playing ? "opacity-100" : "opacity-0",
          )}
        />
      )}
      <ScrubFrame frame={scrub?.frame ?? null} />
      {video.virtual ? (
        <VideoThumbnailDuration className="right-1 bottom-1 px-1 py-0 text-xs font-normal">
          <VirtualMark id={markId} />
        </VideoThumbnailDuration>
      ) : (
        duration !== "" && (
          <VideoThumbnailDuration className="right-1 bottom-1 px-1 py-0 text-xs font-normal">
            {scrubPosition === null
              ? duration
              : t.list.card.scrubTime(formatDuration(scrubPosition.positionMs), duration)}
          </VideoThumbnailDuration>
        )
      )}
      {ratio !== null && (
        <VideoThumbnailProgress
          value={Math.round(ratio * 100)}
          aria-label={t.list.card.watchedRatio}
          // Hide only the visual bar while scrubbing; preserve its value and accessible representation.
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
      {marks}
      {/* Keep the scrub band above duration and progress. */}
      {scrub !== undefined && <ScrubBand scrub={scrub} />}
    </Thumbnail>
  );
}

/** The lg breakpoint enables independently scrolling columns; in theater mode the page scrolls as one. */
const wideScreen = "(min-width: 64rem)";

export function isWideScreen(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia(wideScreen).matches &&
    !theaterMode()
  );
}

/** Track breakpoint and theater changes so pagination switches between scrolling and buttons. */
function useWideScreen(): boolean {
  const [wide, setWide] = useState(isWideScreen);
  useEffect(() => {
    const update = () => setWide(isWideScreen());
    update();
    const unsubscribe = subscribeTheaterMode(update);
    const query =
      typeof window.matchMedia === "function" ? window.matchMedia(wideScreen) : null;
    if (query === null || typeof query.addEventListener !== "function")
      return unsubscribe;
    query.addEventListener("change", update);
    return () => {
      unsubscribe();
      query.removeEventListener("change", update);
    };
  }, []);
  return wide;
}

/** Share one scroll container and offsetTop origin across playlist, continuous-play, and related sections. */
const scrollerClass =
  // Contain scrolling and leave space for hover outlines; align right padding with the header close button.
  "flex flex-col split:relative split:-ml-1.5 split:min-h-0 split:flex-1 split:scrollbar-on-hover split:overflow-y-auto split:overscroll-contain split:pt-1.5 split:pr-3 split:pb-6 split:pl-1.5";

export const cardListClass =
  "grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3 split:grid-cols-1 theater:grid-cols-card-fill";

/** Each section heading remains sticky only while its section is visible. */
export const stickyHeadingClass =
  "flex items-baseline justify-between gap-3 bg-background pb-3 split:sticky split:-top-1.5 split:z-30 split:-mx-1.5 split:-mt-1.5 split:px-1.5 split:pt-1.5";

/** Defaults to the video's URL and return destination; playlist callers preserve playlist context. */
export type MemberTarget = (videoId: number) => { to: string; state: unknown };

/** Paginate related videos by proximity, preserving return/history state. Group members appear first in a separate continuous-play section. */
export default function RelatedVideos({
  state,
  linkState,
  onRetry,
  onLoadMore,
  memberTarget,
  groupContext,
  leading,
}: {
  state: RelatedState;
  linkState: VideoLinkState;
  onRetry: () => void;
  onLoadMore: () => void;
  memberTarget?: MemberTarget;
  /** Preserve group or smart-folder context when loading more members. */
  groupContext?: VideoReadContext;
  /** A leading section that shares this scroll container. */
  leading?: ReactNode;
}) {
  return (
    <div data-related-scroller="" className={scrollerClass}>
      {leading}
      <RelatedSections
        state={state}
        linkState={linkState}
        onRetry={onRetry}
        onLoadMore={onLoadMore}
        memberTarget={memberTarget}
        groupContext={groupContext}
      />
    </div>
  );
}

function RelatedSections({
  state,
  linkState,
  onRetry,
  onLoadMore,
  memberTarget,
  groupContext,
}: {
  state: RelatedState;
  linkState: VideoLinkState;
  onRetry: () => void;
  onLoadMore: () => void;
  memberTarget?: MemberTarget;
  groupContext?: VideoReadContext;
}) {
  if (state.kind === "ready" && state.related.group !== undefined) {
    const group = state.related.group;
    return (
      <GroupedRelated
        // Reset pagination and pending requests when the member, response group, or generation changes.
        key={`${String(state.id)}:${String(group.id)}:${String(state.generation ?? 0)}`}
        currentId={state.id}
        members={group.items}
        offset={group.offset}
        total={group.total}
        items={state.related.items}
        linkState={linkState}
        tail={<RelatedTail state={state} onLoadMore={onLoadMore} />}
        memberTarget={memberTarget}
        groupContext={groupContext}
      />
    );
  }
  // Keep the sentinel when a page becomes empty but more results remain.
  const empty =
    state.kind === "ready" &&
    state.related.items.length === 0 &&
    state.related.nextCursor === undefined;
  return (
    <section
      aria-labelledby={empty ? undefined : "related-heading"}
      className="flex flex-col"
    >
      {!empty && (
        <div className={stickyHeadingClass}>
          <h2 id="related-heading" className="text-sm font-semibold">
            {t.player.related.heading}
          </h2>
        </div>
      )}

      {state.kind === "loading" && (
        <ul aria-hidden="true" className={cardListClass}>
          {Array.from({ length: 6 }, (_, index) => (
            <li key={index}>
              <RelatedCardSkeleton />
            </li>
          ))}
        </ul>
      )}

      {state.kind === "failed" && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-sm text-muted-foreground">{t.player.related.loadFailed}</p>
          <Button variant="ghost" size="sm" onClick={onRetry}>
            {t.common.retry}
          </Button>
        </div>
      )}

      {state.kind === "ready" && !empty && (
        <>
          <ul className={cardListClass}>
            {state.related.items.map((video) => (
              <RelatedItem key={video.id} video={video} linkState={linkState} />
            ))}
          </ul>
          <RelatedTail state={state} onLoadMore={onLoadMore} />
        </>
      )}
    </section>
  );
}

const groupPageSize = 100;

type GroupSide = "before" | "after";

/** Prepend members with scroll compensation on wide screens. Narrow screens use a button to avoid repeatedly loading earlier members as the page opens. */
function GroupedRelated({
  currentId,
  members,
  offset,
  total,
  items,
  linkState,
  tail,
  memberTarget,
  groupContext,
}: {
  currentId: number;
  members: Video[];
  offset: number;
  total: number;
  items: Video[];
  linkState: VideoLinkState;
  tail: ReactNode;
  memberTarget: MemberTarget | undefined;
  groupContext: VideoReadContext | undefined;
}) {
  const wide = useWideScreen();
  const openedGroup = groupContext?.group;
  const openedSmartFolder = groupContext?.smartFolder;
  const [before, setBefore] = useState<Video[]>([]);
  const [after, setAfter] = useState<Video[]>([]);
  const [loading, setLoading] = useState<GroupSide | null>(null);
  const [failed, setFailed] = useState<GroupSide | null>(null);
  const shown = useMemo(() => {
    const seen = new Set(members.map((member) => member.id));
    const unique = (list: Video[]) => list.filter((member) => !seen.has(member.id));
    return [...unique(before), ...members, ...unique(after)];
  }, [after, before, members]);
  const first = offset - before.length;
  const end = offset + members.length + after.length;
  const index = shown.findIndex((member) => member.id === currentId);
  const currentRef = useRef<HTMLLIElement | null>(null);
  const sectionRef = useRef<HTMLElement | null>(null);
  const scrollerOf = () =>
    sectionRef.current?.closest<HTMLElement>("[data-related-scroller]") ?? null;
  const loadingRef = useRef(false);
  const abort = useRef<AbortController | null>(null);
  // Capture height before prepending to keep the visible rows anchored.
  const heightBeforePrepend = useRef<number | null>(null);

  useEffect(() => () => abort.current?.abort(), []);

  const load = useCallback(
    (side: GroupSide) => {
      if (loadingRef.current) return;
      const start = side === "before" ? Math.max(0, first - groupPageSize) : end;
      const limit = side === "before" ? first - start : groupPageSize;
      if (limit <= 0 || (side === "after" && end >= total)) return;
      loadingRef.current = true;
      const controller = new AbortController();
      abort.current = controller;
      setLoading(side);
      setFailed(null);
      listVideoGroupMembers(
        currentId,
        start,
        limit,
        controller.signal,
        openedGroup,
        openedSmartFolder,
      ).then(
        (page) => {
          if (controller.signal.aborted) return;
          loadingRef.current = false;
          setLoading(null);
          if (side === "before") {
            heightBeforePrepend.current = isWideScreen()
              ? (scrollerOf()?.scrollHeight ?? null)
              : null;
            setBefore((previous) => [...page.items, ...previous]);
          } else {
            setAfter((previous) => [...previous, ...page.items]);
          }
        },
        (error: unknown) => {
          if (controller.signal.aborted || isAborted(error)) return;
          loadingRef.current = false;
          setLoading(null);
          setFailed(side);
        },
      );
    },
    [currentId, end, first, openedGroup, openedSmartFolder, total],
  );

  useLayoutEffect(() => {
    const height = heightBeforePrepend.current;
    const scroller = scrollerOf();
    heightBeforePrepend.current = null;
    if (height === null || scroller === null) return;
    scroller.scrollTop += scroller.scrollHeight - height;
  }, [before]);

  const observe = useInfiniteEdge(load);

  // Scroll only the wide-screen list; moving the narrow page would hide the player.
  useLayoutEffect(() => {
    if (!isWideScreen()) return;
    // Center within the container so playlist sections do not obscure neighboring members.
    currentRef.current?.scrollIntoView?.({ block: "center" });
  }, [currentId]);

  return (
    <section ref={sectionRef} aria-labelledby="group-heading" className="flex flex-col">
      <div className="flex flex-col">
        <div className={stickyHeadingClass}>
          <h2 id="group-heading" className="text-sm font-semibold">
            {t.player.related.group}
          </h2>
          {index >= 0 && (
            <span className="text-xs text-muted-foreground tabular-nums">
              {t.player.related.position(first + index + 1, total)}
            </span>
          )}
        </div>
        {first > 0 &&
          (wide && failed !== "before" ? (
            <div ref={observe("before")} aria-hidden="true" className="h-px" />
          ) : (
            <GroupEdge
              label={
                failed === "before" ? t.common.retry : t.player.related.earlier(first)
              }
              failed={failed === "before"}
              disabled={loading === "before"}
              onClick={() => load("before")}
            />
          ))}
        <ul className={cardListClass}>
          {shown.map((member, position) =>
            member.id === currentId ? (
              <CurrentMember
                key={member.id}
                ref={currentRef}
                video={member}
                position={first + position + 1}
              />
            ) : (
              <MemberItem
                key={member.id}
                video={member}
                position={first + position + 1}
                linkState={linkState}
                memberTarget={memberTarget}
              />
            ),
          )}
        </ul>
        {end < total &&
          (failed === "after" ? (
            <GroupEdge
              label={t.common.retry}
              failed
              disabled={loading === "after"}
              onClick={() => load("after")}
            />
          ) : (
            <div ref={observe("after")} aria-hidden="true" className="h-px" />
          ))}
      </div>
      {items.length === 0 && tail}
      {items.length > 0 && (
        <>
          <div className="pt-5 pb-2">
            <Separator decorative={false} />
          </div>
          <section aria-labelledby="related-heading" className="flex flex-col">
            <div className={stickyHeadingClass}>
              <h2 id="related-heading" className="text-sm font-semibold">
                {t.player.related.heading}
              </h2>
            </div>
            <ul className={cardListClass}>
              {items.map((video) => (
                <RelatedItem key={video.id} video={video} linkState={linkState} />
              ))}
            </ul>
            {tail}
          </section>
        </>
      )}
    </section>
  );
}

/** Trigger near an edge, using container-clipped intersections for both page and container scrolling. */
function useInfiniteEdge<Side extends string>(onEdge: (side: Side) => void) {
  const onEdgeRef = useRef(onEdge);
  useEffect(() => {
    onEdgeRef.current = onEdge;
  });
  const observers = useRef(new Map<Side, IntersectionObserver>());
  useEffect(() => {
    const current = observers.current;
    return () => {
      for (const observer of current.values()) observer.disconnect();
      current.clear();
    };
  }, []);
  return useCallback(
    (side: Side) => (element: HTMLDivElement | null) => {
      observers.current.get(side)?.disconnect();
      observers.current.delete(side);
      if (element === null || typeof IntersectionObserver === "undefined") return;
      const observer = new IntersectionObserver(
        (entries) => {
          if (entries.some((entry) => entry.isIntersecting)) onEdgeRef.current(side);
        },
        { rootMargin: "600px 0px" },
      );
      observer.observe(element);
      observers.current.set(side, observer);
    },
    [],
  );
}

function RelatedTail({
  state,
  onLoadMore,
}: {
  state: Extract<RelatedState, { kind: "ready" }>;
  onLoadMore: () => void;
}) {
  const observe = useInfiniteEdge<"related">(onLoadMore);
  if (state.related.nextCursor === undefined) return null;
  if (state.more === "failed") {
    return (
      <GroupEdge label={t.common.retry} failed disabled={false} onClick={onLoadMore} />
    );
  }
  return (
    <>
      {state.more === "loading" && (
        <div aria-hidden="true" className="pt-4">
          <RelatedCardSkeleton />
        </div>
      )}
      <div ref={observe("related")} aria-hidden="true" className="h-px" />
    </>
  );
}

function GroupEdge({
  label,
  failed,
  disabled,
  onClick,
}: {
  label: string;
  failed: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <div className="flex flex-col items-start gap-1 py-2">
      {failed && (
        <p className="text-sm text-muted-foreground">{t.player.related.moreFailed}</p>
      )}
      <Button variant="ghost" size="sm" onClick={onClick} disabled={disabled}>
        {label}
      </Button>
    </div>
  );
}

export function CardNumber({ position }: { position: number }) {
  return (
    <VideoThumbnailMark corner="top-start" aria-hidden="true">
      <span className="rounded-sm bg-overlay px-1.5 text-xs text-foreground tabular-nums">
        {formatNumber(position)}
      </span>
    </VideoThumbnailMark>
  );
}

function MemberTitle({ video }: { video: Video }) {
  const watched = video.progress?.completed === true;
  return (
    <span className="flex min-w-0 items-start gap-1">
      <span
        className={cn(
          "line-clamp-2 min-w-0 text-sm font-medium wrap-anywhere",
          watched ? "text-muted-foreground" : "text-foreground",
        )}
      >
        {video.title}
      </span>
      {watched && (
        <Check
          className="mt-0.5 size-4 shrink-0 text-success"
          strokeWidth={2.5}
          aria-hidden="true"
        />
      )}
    </span>
  );
}

export function memberLinkLabel(video: Video): string | UiText {
  const label = videoLinkLabel(video);
  return video.progress?.completed === true ? t.player.related.watchedLink(label) : label;
}

/** Pause preview while scrubbing; resume only inside the link. Leaving or resizing releases the preview and aborts pending requests. */
export function useRelatedScrub(video: Video, preview: HoverPreview) {
  const { suspendPreview, resumePreview, onPointerLeave: releasePreview } = preview;
  const scrub = useScrubPreview({
    video,
    onSuspend: suspendPreview,
    onResume: resumePreview,
  });
  const { leaveCard, cardRef } = scrub;
  const linkRef = useRef<HTMLAnchorElement | null>(null);
  const setLink = useCallback(
    (element: HTMLAnchorElement | null) => {
      linkRef.current = element;
      cardRef(element);
    },
    [cardRef],
  );

  const bandLeave = scrub.bandHandlers.onPointerLeave;
  const onBandLeave = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const next = event.relatedTarget;
      const link = linkRef.current;
      if (link !== null && !(next instanceof Node && link.contains(next))) {
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

  useEffect(() => {
    window.addEventListener("resize", release);
    return () => window.removeEventListener("resize", release);
  }, [release]);

  const band: ScrubPreview = {
    ...scrub,
    bandHandlers: { ...scrub.bandHandlers, onPointerLeave: onBandLeave },
  };
  return { scrub: band, release, setLink };
}

function MemberItem({
  video,
  position,
  linkState,
  memberTarget,
}: {
  video: Video;
  position: number;
  linkState: VideoLinkState;
  memberTarget: MemberTarget | undefined;
}) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  const target = memberTarget?.(video.id) ?? {
    to: `/videos/${String(video.id)}`,
    state: linkState,
  };
  return (
    <li>
      <Link
        ref={setLink}
        to={target.to}
        state={target.state}
        aria-label={memberLinkLabel(video)}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex flex-col gap-2 rounded-lg p-1.5 transition-colors hover:bg-accent"
      >
        <VideoThumbnail
          video={video}
          preview={preview}
          scrub={scrub}
          marks={<CardNumber position={position} />}
        />
        <MemberTitle video={video} />
      </Link>
    </li>
  );
}

/** The current member is not interactive and has no hover treatment. */
function CurrentMember({
  ref,
  video,
  position,
}: {
  ref: Ref<HTMLLIElement>;
  video: Video;
  position: number;
}) {
  const watched = video.progress?.completed === true;
  return (
    // Leave scroll margins for the sticky heading and card edges.
    <li ref={ref} aria-current="true" className="scroll-mt-12 scroll-mb-6">
      <div className="-m-1.5 flex flex-col gap-2 rounded-lg bg-accent p-1.5 ring-2 ring-primary ring-inset">
        <VideoThumbnail video={video} marks={<CardNumber position={position} />} />
        <span className="sr-only">{t.player.related.nowPlaying}</span>
        <MemberTitle video={video} />
        {watched && <span className="sr-only">{t.player.related.watched}</span>}
      </div>
    </li>
  );
}

function RelatedItem({ video, linkState }: { video: Video; linkState: VideoLinkState }) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  return (
    <li>
      <Link
        ref={setLink}
        to={`/videos/${String(video.id)}`}
        state={linkState}
        aria-label={videoLinkLabel(video)}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex flex-col gap-2 rounded-lg p-1.5 transition-colors hover:bg-accent"
      >
        <VideoThumbnail
          video={video}
          className="w-full"
          preview={preview}
          scrub={scrub}
        />
        <span className="line-clamp-2 min-w-0 text-sm font-medium wrap-anywhere">
          {video.title}
        </span>
      </Link>
    </li>
  );
}

function RelatedCardSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      <Skeleton className="aspect-video w-full" />
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-4 w-2/3" />
    </div>
  );
}
