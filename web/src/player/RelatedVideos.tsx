import { Check, ImageOff } from "lucide-react";
import {
  type PointerEvent,
  type Ref,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import type { RelatedState } from "../api/useVideoDetail";
import { type HoverPreview, useHoverPreview } from "./useHoverPreview";
import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, isNarrowVideo, watchedRatio } from "../lib/format";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import Button from "../ui/Button";
import {
  ScrubBand,
  ScrubFrame,
  type ScrubPreview,
  useScrubPreview,
} from "../ui/ScrubPreview";
import Skeleton from "../ui/Skeleton";

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
    <div
      className={cn(
        "relative aspect-video shrink-0 overflow-hidden rounded-md bg-surface",
        className,
      )}
    >
      {video.thumbnailUrl !== undefined && isNarrowVideo(video) && (
        <ThumbnailBackdrop src={video.thumbnailUrl} />
      )}
      {video.thumbnailUrl !== undefined ? (
        <img
          src={video.thumbnailUrl}
          alt=""
          loading="lazy"
          decoding="async"
          className={cn(
            "relative h-full w-full object-contain",
            preview?.playing === true && "opacity-0",
          )}
        />
      ) : (
        <div className="flex h-full w-full flex-col items-center justify-center gap-1 text-fg-subtle">
          <ImageOff className="size-5" strokeWidth={1.5} aria-hidden="true" />
          <span className="text-xs">
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
            "absolute inset-0 h-full w-full object-contain",
            preview.playing ? "opacity-100" : "opacity-0",
          )}
        />
      )}
      <ScrubFrame frame={scrub?.frame ?? null} />
      {duration !== "" && (
        <span className="absolute right-1 bottom-1 rounded-sm bg-overlay px-1 text-xs text-fg tabular-nums">
          {scrubPosition === null
            ? duration
            : t.list.card.scrubTime(formatDuration(scrubPosition.positionMs), duration)}
        </span>
      )}
      {ratio !== null && (
        <span
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(ratio * 100)}
          aria-label={t.list.card.watchedRatio}
          // 帯にいる間は見た目だけ隠し、値と読み上げは保つ（R-6）。
          className={cn(
            "absolute inset-x-0 bottom-0 h-[3px] bg-fg-subtle/50",
            scrubPosition !== null && "opacity-0",
          )}
        >
          <span
            className="block h-full bg-primary"
            style={{ width: `${String(Math.round(ratio * 100))}%` }}
          />
        </span>
      )}
      {scrubPosition !== null && (
        <span
          aria-hidden="true"
          data-scrub-bar=""
          className="pointer-events-none absolute inset-x-0 bottom-0 h-[3px] bg-fg-subtle/50"
        >
          <span
            className="block h-full bg-fg"
            style={{ width: `${String(scrubPosition.ratio * 100)}%` }}
          />
        </span>
      )}
      {/* 帯は長さの表示とバーより前に置く（R-5）。 */}
      {scrub !== undefined && <ScrubBand scrub={scrub} />}
    </div>
  );
}

/** 広い画面（`lg` 以上）。左右の列がそれぞれ中でスクロールする幅である。 */
const wideScreen = "(min-width: 64rem)";

function isWideScreen(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(wideScreen).matches;
}

/** 関連動画の並びの入れ物。広い画面では並びだけを中でスクロールさせる。 */
const scrollerClass =
  // 端まで来てもページへは送らない。行の hover の面と輪郭が切れないよう、はみ出す分だけ
  // 内側に余白を取る。
  "lg:-mx-1.5 lg:min-h-0 lg:scrollbar-on-hover lg:overflow-y-auto lg:overscroll-contain lg:px-1.5 lg:pt-1.5 lg:pb-6";

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
    return (
      <GroupedRelated
        currentId={state.id}
        members={state.related.group.items}
        items={state.related.items}
        backTo={backTo}
      />
    );
  }
  const empty = state.kind === "ready" && state.related.items.length === 0;
  return (
    <section
      aria-labelledby={empty ? undefined : "related-heading"}
      className="flex flex-col gap-3 lg:min-h-0 lg:flex-1"
    >
      {!empty && (
        <h2 id="related-heading" className="text-sm font-semibold text-fg">
          {t.player.related.heading}
        </h2>
      )}

      {state.kind === "loading" && (
        <ul aria-hidden="true" className="flex flex-col gap-3">
          {Array.from({ length: 6 }, (_, index) => (
            <li key={index} className="flex gap-3">
              <Skeleton className="aspect-video w-40 shrink-0" />
              <div className="flex flex-1 flex-col gap-2 pt-1">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            </li>
          ))}
        </ul>
      )}

      {state.kind === "failed" && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-sm text-fg-muted">{t.player.related.loadFailed}</p>
          <Button variant="ghost" size="sm" onClick={onRetry}>
            {t.common.retry}
          </Button>
        </div>
      )}

      {state.kind === "ready" && !empty && (
        <ul className={cn("flex flex-col gap-3", scrollerClass)}>
          {state.related.items.map((video) => (
            <RelatedItem key={video.id} video={video} backTo={backTo} />
          ))}
        </ul>
      )}
    </section>
  );
}

/**
 * GroupedRelated は、グループのメンバーを開いたときの列である。見出し「続けて再生」と
 * 何本目かを上に留め、その下のメンバーの並び・境目・関連動画を1つの入れ物でスクロールさせる。
 * メンバーの並びと関連動画は別々の `ul` にし、読み上げでも境目が分かるようにする。
 */
function GroupedRelated({
  currentId,
  members,
  items,
  backTo,
}: {
  currentId: number;
  members: Video[];
  items: Video[];
  backTo: string;
}) {
  const index = members.findIndex((member) => member.id === currentId);
  const currentRef = useRef<HTMLLIElement | null>(null);

  // 広い画面では、開いたとき（別のメンバーへ移ったときを含む）に今のメンバーの行を入れ物の
  // 中で見える位置へ動かす。ページと左の列は動かさない。狭い画面ではページごと動いて
  // プレイヤーが画面の上から消えるので、動かさない（Edge Case「大きなグループ」）。
  useLayoutEffect(() => {
    if (!isWideScreen()) return;
    currentRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [currentId]);

  return (
    <section
      aria-labelledby="group-heading"
      className="flex flex-col gap-3 lg:min-h-0 lg:flex-1"
    >
      <div className="flex items-baseline justify-between gap-3">
        <h2 id="group-heading" className="text-sm font-semibold text-fg">
          {t.player.related.group}
        </h2>
        {index >= 0 && (
          <span className="text-xs text-fg-muted tabular-nums">
            {t.player.related.position(index + 1, members.length)}
          </span>
        )}
      </div>
      <div data-related-scroller="" className={cn("flex flex-col", scrollerClass)}>
        <ul className="flex flex-col gap-3">
          {members.map((member, position) =>
            member.id === currentId ? (
              <CurrentMember
                key={member.id}
                ref={currentRef}
                video={member}
                position={position + 1}
              />
            ) : (
              <MemberItem
                key={member.id}
                video={member}
                position={position + 1}
                backTo={backTo}
              />
            ),
          )}
        </ul>
        {items.length > 0 && (
          <>
            <div className="pt-5 pb-2">
              <hr className="border-t border-border" />
            </div>
            <section aria-labelledby="related-heading" className="flex flex-col gap-3">
              <h2 id="related-heading" className="text-sm font-semibold text-fg">
                {t.player.related.heading}
              </h2>
              <ul className="flex flex-col gap-3">
                {items.map((video) => (
                  <RelatedItem key={video.id} video={video} backTo={backTo} />
                ))}
              </ul>
            </section>
          </>
        )}
      </div>
    </section>
  );
}

/** 並びの番号。今の位置と残りが数えなくても分かるよう、サムネイルの左に置く。 */
function MemberNumber({ position }: { position: number }) {
  return (
    <span
      aria-hidden="true"
      className="w-5 shrink-0 pt-0.5 text-right text-xs text-fg-muted tabular-nums"
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
          "line-clamp-2 min-w-0 text-sm font-medium [overflow-wrap:anywhere]",
          watched ? "text-fg-muted" : "text-fg",
        )}
      >
        {video.title}
      </span>
      {watched && (
        <Check
          className="mt-0.5 size-3.5 shrink-0 text-success"
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
        className="-m-1.5 flex gap-3 rounded-lg p-1.5 transition-colors hover:bg-hover-wash"
      >
        <MemberNumber position={position} />
        <VideoThumbnail video={video} className="w-40" preview={preview} scrub={scrub} />
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
      <div className="-m-1.5 flex gap-3 rounded-lg border-l-2 border-primary bg-active-wash p-1.5 pl-1">
        <MemberNumber position={position} />
        <VideoThumbnail video={video} className="w-40" />
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
        className="-m-1.5 flex gap-3 rounded-lg p-1.5 transition-colors hover:bg-hover-wash"
      >
        <VideoThumbnail video={video} className="w-40" preview={preview} scrub={scrub} />
        <span className="line-clamp-2 min-w-0 text-sm font-medium text-fg [overflow-wrap:anywhere]">
          {video.title}
        </span>
      </Link>
    </li>
  );
}
