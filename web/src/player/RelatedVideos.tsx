import { Check, ImageOff } from "lucide-react";
import {
  type PointerEvent,
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
import type { RelatedState } from "../api/useVideoDetail";
import { type HoverPreview, useHoverPreview } from "./useHoverPreview";
import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, isNarrowVideo, watchedRatio } from "../lib/format";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import {
  ScrubBand,
  ScrubFrame,
  type ScrubPreview,
  useScrubPreview,
} from "../ui/ScrubPreview";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import {
  VideoThumbnail as Thumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
  VideoThumbnailProgress,
} from "../ui/VideoThumbnail";

/**
 * videoLinkLabel は関連動画と「次の動画」のリンクの読み上げ名である。題名と長さだけにし、
 * 中の進捗バーの割合が名前に混ざらないようにする。進捗バー自体は読み上げに残す。
 */
export function videoLinkLabel(video: Video): string {
  const duration = formatDuration(video.durationMs);
  return duration === ""
    ? video.title
    : t.player.related.videoLink(video.title, duration);
}

/**
 * VideoThumbnail は関連動画と「次の動画」のサムネイルである。無いときは一覧のカードと
 * 同じ代わりの表示にし、右下に長さ、途中まで見た動画だけ下端に進捗バーを出す。
 *
 * scrub を渡すと下端にスクラブの帯を置き、帯にいる間はコマを出して長さの表示とバーを
 * スクラブ位置に差し替える（specs/032-card-scrub-preview/ui-design.md）。
 */
export function VideoThumbnail({
  video,
  className,
  preview,
  scrub,
}: {
  video: Video;
  className?: string;
  /** マウスを乗せたときの一覧用プレビュー。関連動画の列だけが渡す。 */
  preview?: HoverPreview;
  /** スクラブの帯。関連動画の列のリンクだけが渡す。 */
  scrub?: ScrubPreview;
}) {
  const duration = formatDuration(video.durationMs);
  const ratio = watchedRatio(video);
  // 帯にいる間のポインタの位置（ui-design.md「Time and bar」）。
  const scrubPosition = scrub?.position ?? null;
  return (
    <Thumbnail className={cn("shrink-0", className)}>
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
      {duration !== "" && (
        <VideoThumbnailDuration>
          {scrubPosition === null
            ? duration
            : t.list.card.scrubTime(formatDuration(scrubPosition.positionMs), duration)}
        </VideoThumbnailDuration>
      )}
      {ratio !== null && (
        <VideoThumbnailProgress
          value={Math.round(ratio * 100)}
          aria-label={t.list.card.watchedRatio}
          // 帯にいる間は見た目だけ隠し、値と読み上げは保つ（R-6）。
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
      {/* 帯は長さの表示とバーより前に置く（R-5）。 */}
      {scrub !== undefined && <ScrubBand scrub={scrub} />}
    </Thumbnail>
  );
}

/** 広い画面（`lg` 以上）。左右の列がそれぞれ中でスクロールする幅である。 */
const wideScreen = "(min-width: 64rem)";

function isWideScreen(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(wideScreen).matches;
}

/**
 * useWideScreen は広い画面かを返し、窓の幅や向きが `lg` をまたいで変わったら描き直す。
 * 描くときに一度だけ調べると、またいだ後も前の幅の読み足し方（スクロールかボタンか）が残る。
 */
function useWideScreen(): boolean {
  const [wide, setWide] = useState(isWideScreen);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia(wideScreen);
    if (typeof query.addEventListener !== "function") return;
    const update = () => setWide(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);
  return wide;
}

/**
 * scrollContainer は、広い画面で関連動画の列ごとスクロールする入れ物（詳細ページの情報欄、
 * web/src/ui/patterns/detail-page.tsx）を返す。前のメンバーを読み足したときに、見ている行が
 * 動かないようスクロールの位置を補うのに使う。
 */
function scrollContainer(element: HTMLElement | null): HTMLElement | null {
  return element?.closest<HTMLElement>('[data-slot="detail-page-aside"]') ?? null;
}

/** 関連動画の行のサムネイルの幅。行の幅の半分で、card-0 を超えない。 */
const thumbnailWidth = "w-1/2 max-w-card-0";

/**
 * RelatedVideos は関連動画の列である（要件 15）。
 *
 * 関連動画が 0 件のときは見出しと並びを出さない（閉じる × は見出しの帯 VideoHeader にある）。
 * 各項目はサムネイル・長さ・題名だけで、追加日時などの文字は出さない。
 * リンクは最初の戻り先を引き継ぐ（plan の Structural Decisions 10）。
 *
 * 基準の動画がグループのメンバーなら、列の上位に「続けて再生」としてメンバーの並びを
 * 出し、境目の下に関連動画を出す（specs/017-folder-groups/ui-design.md「Member list」）。
 */
export default function RelatedVideos({
  state,
  backTo,
  onRetry,
}: {
  state: RelatedState;
  backTo: string;
  onRetry: () => void;
}) {
  if (state.kind === "ready" && state.related.group !== undefined) {
    const group = state.related.group;
    return (
      <GroupedRelated
        // 別のメンバーへ移ったら、読み足した分を捨てて新しい窓から始める。
        key={state.id}
        currentId={state.id}
        members={group.items}
        offset={group.offset}
        total={group.total}
        items={state.related.items}
        backTo={backTo}
      />
    );
  }
  // 関連動画が 0 件のときは節ごと出さない。
  if (state.kind === "ready" && state.related.items.length === 0) return null;
  return (
    <PageSection title={t.player.related.heading}>
      {state.kind === "loading" && (
        <ul aria-hidden="true" className="flex flex-col gap-3">
          {Array.from({ length: 6 }, (_, index) => (
            <li key={index} className="flex gap-3">
              <Skeleton className={cn("aspect-video shrink-0", thumbnailWidth)} />
              <div className="flex flex-1 flex-col gap-2 pt-1">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            </li>
          ))}
        </ul>
      )}

      {state.kind === "failed" && (
        <ErrorState
          title={t.player.related.loadFailed}
          retryLabel={t.common.retry}
          onRetry={onRetry}
        />
      )}

      {state.kind === "ready" && (
        <ul className="flex flex-col gap-3">
          {state.related.items.map((video) => (
            <RelatedItem key={video.id} video={video} backTo={backTo} />
          ))}
        </ul>
      )}
    </PageSection>
  );
}

/** groupPageSize は、窓の外のメンバーを1回に読む本数である。 */
const groupPageSize = 100;

/** 窓の外を読む向き。 */
type GroupSide = "before" | "after";

/**
 * GroupedRelated は、グループのメンバーを開いたときの列である。見出し「続けて再生」と
 * 何本目かを上に留め、その下のメンバーの並び・境目・関連動画を1つの入れ物でスクロールさせる。
 * メンバーの並びと関連動画は別々の `ul` にし、読み上げでも境目が分かるようにする。
 *
 * 関連動画の応答のメンバーは、今の動画を中ほどに置いた窓（`offset` から）だけである。
 * 窓の後ろはスクロールで続きを読み足す。前は、広い画面では入れ物を上へスクロールすると
 * 読み足し（見ている行が動かないようスクロールの位置を補う）、狭い画面ではボタンで読む。
 * 狭い画面ではページごと動くので、並びの先頭が見えた時点で読み足すと、開いた直後から
 * 前のメンバーを次々に読んでしまう（issue 674）。
 */
function GroupedRelated({
  currentId,
  members,
  offset,
  total,
  items,
  backTo,
}: {
  currentId: number;
  members: Video[];
  offset: number;
  total: number;
  items: Video[];
  backTo: string;
}) {
  const wide = useWideScreen();
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
  const listRef = useRef<HTMLDivElement | null>(null);
  const loadingRef = useRef(false);
  const abort = useRef<AbortController | null>(null);
  // 前を読み足す直前の入れ物の高さ。読み足した後、見ている行が動かないよう補う。
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
      listVideoGroupMembers(currentId, start, limit, controller.signal).then(
        (page) => {
          if (controller.signal.aborted) return;
          loadingRef.current = false;
          setLoading(null);
          if (side === "before") {
            heightBeforePrepend.current = isWideScreen()
              ? (scrollContainer(listRef.current)?.scrollHeight ?? null)
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
    [currentId, end, first, total],
  );

  // 前を読み足したら、増えた高さだけ入れ物を下へずらし、見ていた行を同じ位置に残す。
  useLayoutEffect(() => {
    const height = heightBeforePrepend.current;
    const scroller = scrollContainer(listRef.current);
    heightBeforePrepend.current = null;
    if (height === null || scroller === null) return;
    scroller.scrollTop += scroller.scrollHeight - height;
  }, [before]);

  // 端の目印が見えそうになったら続きを読む。前の目印は広い画面だけに置く。
  const observe = useInfiniteEdge(load);

  // 広い画面では、開いたとき（別のメンバーへ移ったときを含む）に今のメンバーの行を情報欄の
  // 中で見える位置へ動かす。ページと主領域は動かさない。狭い画面ではページごと動いて
  // プレイヤーが画面の上から消えるので、動かさない（Edge Case「大きなグループ」）。
  useLayoutEffect(() => {
    if (!isWideScreen()) return;
    currentRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [currentId]);

  return (
    <>
      <PageSection
        title={t.player.related.group}
        description={
          index >= 0 ? (
            <span className="tabular-nums">
              {t.player.related.position(first + index + 1, total)}
            </span>
          ) : undefined
        }
      >
        <div ref={listRef} className="flex flex-col">
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
          <ul className="flex flex-col gap-3">
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
                  backTo={backTo}
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
      </PageSection>
      {items.length > 0 && (
        <PageSection title={t.player.related.heading}>
          <ul className="flex flex-col gap-3">
            {items.map((video) => (
              <RelatedItem key={video.id} video={video} backTo={backTo} />
            ))}
          </ul>
        </PageSection>
      )}
    </>
  );
}

/**
 * useInfiniteEdge は、端の目印（要素）が画面の近くに入ったら onEdge(side) を呼ぶ ref を返す。
 * 入れ物の中のスクロールでも、ページのスクロールでも同じに働く（交差は入れ物で切り取って
 * 判定される）。
 */
function useInfiniteEdge(onEdge: (side: GroupSide) => void) {
  const onEdgeRef = useRef(onEdge);
  useEffect(() => {
    onEdgeRef.current = onEdge;
  });
  const observers = useRef(new Map<GroupSide, IntersectionObserver>());
  useEffect(() => {
    const current = observers.current;
    return () => {
      for (const observer of current.values()) observer.disconnect();
      current.clear();
    };
  }, []);
  return useCallback(
    (side: GroupSide) => (element: HTMLDivElement | null) => {
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

/** GroupEdge は窓の外を読むボタン（狭い画面の前、読み足しの失敗のやり直し）である。 */
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

/** 並びの番号。今の位置と残りが数えなくても分かるよう、サムネイルの左に置く。 */
function MemberNumber({ position }: { position: number }) {
  return (
    <span
      aria-hidden="true"
      className="w-5 shrink-0 pt-0.5 text-right text-xs text-muted-foreground tabular-nums"
    >
      {formatNumber(position)}
    </span>
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

/** memberLinkLabel はメンバーの行の読み上げ名である。視聴済みならそれを続ける。 */
export function memberLinkLabel(video: Video): string | UiText {
  const label = videoLinkLabel(video);
  return video.progress?.completed === true ? t.player.related.watchedLink(label) : label;
}

/**
 * useRelatedScrub は関連動画の行のスクラブの帯をループ再生（useHoverPreview）とつなぐ
 * （specs/032-card-scrub-preview/research.md R-2、ui-design.md「Pointer rules」）。
 *
 * - 帯への出入りでループを一時停止・再開する。帯からリンクの外へ出たときは再開せず、
 *   リンクの pointerleave の解放に任せる。
 * - リンクから出たら、帯から出たのと同じに戻し、進行中の取得を打ち切る。
 * - 窓の大きさが変わったら、一覧の usePreviewCoordination と同じくループを解放し、
 *   帯から出たのと同じに戻す（ui-design.md「Responsive」）。
 */
function useRelatedScrub(video: Video, preview: HoverPreview) {
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
  backTo,
}: {
  video: Video;
  position: number;
  backTo: string;
}) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  return (
    <li>
      <Link
        ref={setLink}
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={memberLinkLabel(video)}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex gap-3 rounded-md p-1.5 transition-colors hover:bg-accent"
      >
        <MemberNumber position={position} />
        <VideoThumbnail
          video={video}
          className={thumbnailWidth}
          preview={preview}
          scrub={scrub}
        />
        <MemberTitle video={video} />
      </Link>
    </li>
  );
}

/** CurrentMember は今見ているメンバーの行である。押せないので、リンクにせず hover でも変えない。 */
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
    // 入れ物の端にぴったり付かないよう、動かすときは上下に少し間を残す。
    <li ref={ref} aria-current="true" className="scroll-my-6">
      {/* 左の線の太さの分だけ左の余白を減らし、番号とサムネイルの位置を他の行とそろえる。 */}
      <div className="-m-1.5 flex gap-3 rounded-md border-l-2 border-primary bg-primary-soft p-1.5 pl-1">
        <MemberNumber position={position} />
        <VideoThumbnail video={video} className={thumbnailWidth} />
        <span className="sr-only">{t.player.related.nowPlaying}</span>
        <MemberTitle video={video} />
        {watched && <span className="sr-only">{t.player.related.watched}</span>}
      </div>
    </li>
  );
}

function RelatedItem({ video, backTo }: { video: Video; backTo: string }) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  return (
    <li>
      <Link
        ref={setLink}
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={videoLinkLabel(video)}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex gap-3 rounded-md p-1.5 transition-colors hover:bg-accent"
      >
        <VideoThumbnail
          video={video}
          className={thumbnailWidth}
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
