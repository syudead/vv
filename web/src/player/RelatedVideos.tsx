import { Check, ImageOff } from "lucide-react";
import {
  defaultRangeExtractor,
  type Range,
  useVirtualizer,
  useWindowVirtualizer,
  type VirtualItem,
  type Virtualizer,
} from "@tanstack/react-virtual";
import {
  type FocusEvent,
  type PointerEvent,
  type Ref,
  type RefObject,
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
            className="block h-full bg-accent"
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
 *
 * 読み込んだメンバーは数千本になりうるので、並びは見えている行の近くだけを描く
 * （issue 675）。広い画面は入れ物、狭い画面は文書を基準にするので、幅ごとに別の
 * 部品（WideMemberList・PageMemberList）にする。
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
  const scrollerRef = useRef<HTMLDivElement | null>(null);
  // 並びが描く今のメンバーの行へスクロールする口（何本目かのボタンが呼ぶ）。
  const jump = useRef<(() => void) | null>(null);
  // 開いたときの位置合わせを済ませたか。幅が lg をまたいで並びを描き直しても繰り返さない。
  const opened = useRef(false);
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
              ? (scrollerRef.current?.scrollHeight ?? null)
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
  // 並びは仮想化しても全件の高さを持つ（読み足した行は見込みの高さで足される）ので、
  // 入れ物の高さの差がそのまま増えた分になる。
  useLayoutEffect(() => {
    const height = heightBeforePrepend.current;
    const scroller = scrollerRef.current;
    heightBeforePrepend.current = null;
    if (height === null || scroller === null) return;
    scroller.scrollTop += scroller.scrollHeight - height;
  }, [before]);

  // 端の目印が見えそうになったら続きを読む。前の目印は広い画面だけに置く。
  const observe = useInfiniteEdge(load);

  // 並びの上の前の端。並びの上端の位置はこれで変わる。
  const beforeEdge: BeforeEdge =
    first === 0 ? "none" : failed === "before" ? "retry" : wide ? "sentinel" : "button";
  const listProps: MemberListProps = {
    beforeEdge,
    shown,
    currentIndex: index,
    first,
    total,
    backTo,
    jump,
    opened,
  };

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
          // 今の動画が並びの見えない所にあっても、ここから位置へ戻れる（Edge Case
          // 「大きなグループ」、issue 675）。
          <button
            type="button"
            aria-label={t.player.related.jumpToCurrent(first + index + 1, total)}
            onClick={() => jump.current?.()}
            className="rounded-sm text-xs text-fg-muted tabular-nums hover:underline focus-visible:underline"
          >
            {t.player.related.position(first + index + 1, total)}
          </button>
        )}
      </div>
      <div
        ref={scrollerRef}
        data-related-scroller=""
        className={cn("flex flex-col", scrollerClass)}
      >
        {beforeEdge !== "none" &&
          (beforeEdge === "sentinel" ? (
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
        {wide ? <WideMemberList {...listProps} /> : <PageMemberList {...listProps} />}
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

/** memberRowHeight はメンバーの1行の見込みの高さ（サムネイル `w-40` の 16:9、px）である。 */
const memberRowHeight = 90;
/** memberRowGap は行の間（`gap-3`、px）である。 */
const memberRowGap = 12;
/**
 * memberOverscan は、見えている行の前後に余分に描く行の数である。Tab で次の行へ
 * 進むとき、その行が描かれているようにする。
 */
const memberOverscan = 4;
/** memberScrollMargin は、行へ動かすときに入れ物や画面の端へ残す間（px）である。 */
const memberScrollMargin = 24;

/** BeforeEdge は並びの上の前の端（無い・目印・ボタン・やり直し）である。 */
type BeforeEdge = "none" | "sentinel" | "button" | "retry";

/** MemberListProps はメンバーの並び（広い画面・狭い画面）が受け取るものである。 */
interface MemberListProps {
  /** 並びの上の前の端。入れ替わると並びの上端が動くので測り直す。 */
  beforeEdge: BeforeEdge;
  /** 読み込んだメンバー（窓と読み足した分）。 */
  shown: Video[];
  /** 今のメンバーの shown の中の位置。無ければ -1。 */
  currentIndex: number;
  /** shown の先頭のグループ全体の中の位置（0 始まり）。 */
  first: number;
  /** グループ全体の本数。 */
  total: number;
  backTo: string;
  /** 今のメンバーの行へスクロールする処理を、何本目かのボタンへ渡す口。 */
  jump: RefObject<(() => void) | null>;
  /** 開いたときの位置合わせを済ませたか。 */
  opened: RefObject<boolean>;
}

/**
 * revealMember は index の行が見える位置へスクロールする（既に見えていれば動かない）。
 *
 * scrollToIndex は目標を行の番号で持ち、描いた行を測るたびに番号から位置を
 * 取り直す。前を読み足すと番号がずれ、別の行へ動き直すので、位置を一度だけ求めて
 * そこへ動かす。行の高さはサムネイルで決まり、見込みと測った値がほぼ変わらない。
 */
function revealMember<S extends Element | Window>(
  virtualizer: Virtualizer<S, Element>,
  index: number,
) {
  if (index < 0) return;
  const target = virtualizer.getOffsetForIndex(index, "auto");
  if (target !== undefined) virtualizer.scrollToOffset(target[0]);
}

/**
 * useMemberRange は、描く行の範囲に今のメンバーの行とフォーカスを持つ行を足す。
 * 今のメンバーの行は読み上げの目印（`aria-current`）として、フォーカスを持つ行は
 * スクロールで画面の外へ出てもフォーカスを失わないよう、描き続ける。
 */
function useMemberRange(shown: Video[], currentIndex: number) {
  const [focusedId, setFocusedId] = useState<number | null>(null);
  const focusedIndex =
    focusedId === null ? -1 : shown.findIndex((member) => member.id === focusedId);
  const rangeExtractor = useCallback(
    (range: Range) => {
      const indexes = defaultRangeExtractor(range);
      const extra = [currentIndex, focusedIndex].filter(
        (index, i, all) =>
          index >= 0 &&
          index < range.count &&
          !indexes.includes(index) &&
          all.indexOf(index) === i,
      );
      if (extra.length === 0) return indexes;
      return [...indexes, ...extra].sort((a, b) => a - b);
    },
    [currentIndex, focusedIndex],
  );
  // 行の高さはメンバーごとに覚え、前を読み足して番号がずれても引き継ぐ。
  const getItemKey = useCallback((index: number) => shown[index]?.id ?? index, [shown]);
  const onFocus = (event: FocusEvent<HTMLUListElement>) => {
    const row = (event.target as Element).closest<HTMLElement>("[data-index]");
    const member = row === null ? undefined : shown[Number(row.dataset.index)];
    setFocusedId(member?.id ?? null);
  };
  const onBlur = (event: FocusEvent<HTMLUListElement>) => {
    const next = event.relatedTarget;
    if (next instanceof Node && event.currentTarget.contains(next)) return;
    setFocusedId(null);
  };
  return { rangeExtractor, getItemKey, onFocus, onBlur };
}

/**
 * WideMemberList は広い画面（`lg` 以上）のメンバーの並びである。並びは列の入れ物の
 * 中でスクロールするので、入れ物を基準に、見えている行と前後の数行だけを描く
 * （issue 675）。開いたときは今のメンバーの行を入れ物の中で見える位置へ動かす。
 * ページと左の列は動かさない。
 */
function WideMemberList(props: MemberListProps) {
  const { beforeEdge, shown, currentIndex, jump, opened } = props;
  const listRef = useRef<HTMLUListElement | null>(null);
  // 並びの上端の入れ物の中での位置。上には入れ物の余白と前の端（目印かボタン）がある。
  // 測るまでは null とし、開いたときの位置合わせを待たせる。
  const [margin, setMargin] = useState<number | null>(null);
  const scroller = () =>
    listRef.current?.closest<HTMLElement>("[data-related-scroller]") ?? null;
  // 前の端は読み込みの失敗で目印とやり直しのボタンが入れ替わるので、そのたびに測る。
  useLayoutEffect(() => {
    const list = listRef.current;
    const element = list?.closest<HTMLElement>("[data-related-scroller]");
    if (list === null || element === null || element === undefined) return;
    setMargin(
      list.getBoundingClientRect().top -
        element.getBoundingClientRect().top +
        element.scrollTop,
    );
  }, [beforeEdge]);
  const range = useMemberRange(shown, currentIndex);
  const virtualizer = useVirtualizer({
    count: shown.length,
    getScrollElement: scroller,
    estimateSize: () => memberRowHeight,
    gap: memberRowGap,
    overscan: memberOverscan,
    scrollMargin: margin ?? 0,
    scrollPaddingStart: memberScrollMargin,
    scrollPaddingEnd: memberScrollMargin,
    rangeExtractor: range.rangeExtractor,
    getItemKey: range.getItemKey,
  });
  const reveal = useCallback(
    () => revealMember(virtualizer, currentIndex),
    [virtualizer, currentIndex],
  );
  useEffect(() => {
    jump.current = reveal;
  }, [jump, reveal]);
  // 開いたとき（別のメンバーへ移ったときを含む）に一度だけ動かす。
  useLayoutEffect(() => {
    if (margin === null || opened.current) return;
    opened.current = true;
    reveal();
  }, [margin, opened, reveal]);
  return (
    <MemberRows
      {...props}
      listRef={listRef}
      items={virtualizer.getVirtualItems()}
      totalSize={virtualizer.getTotalSize()}
      scrollMargin={virtualizer.options.scrollMargin}
      measure={virtualizer.measureElement}
      onFocus={range.onFocus}
      onBlur={range.onBlur}
    />
  );
}

/** navbarHeight は動画ページの上に留まる帯の高さ（px）である。 */
function navbarHeight(): number {
  return (
    parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue("--spacing-navbar"),
    ) || 52
  );
}

/**
 * PageMemberList は狭い画面（`lg` 未満）のメンバーの並びである。ページごと
 * スクロールするので、文書を基準に、見えている行と前後の数行だけを描く（issue 675）。
 * 開いたときは動かさない。ページごと動いてプレイヤーが画面の上から消える
 * （Edge Case「大きなグループ」）。
 */
function PageMemberList(props: MemberListProps) {
  const { beforeEdge, shown, currentIndex, jump, opened } = props;
  const listRef = useRef<HTMLUListElement | null>(null);
  // 並びの上端の文書の中での位置。上のプレイヤーや題名の高さで動く。
  const [margin, setMargin] = useState(0);
  const [paddingStart] = useState(() => navbarHeight() + memberScrollMargin);
  const measure = useCallback(() => {
    const list = listRef.current;
    if (list !== null) setMargin(list.getBoundingClientRect().top + window.scrollY);
  }, []);
  // 前の端（ボタンとやり直し）が入れ替わったとき、プレイヤーの大きさや題名の折り返しで
  // 文書の大きさが変わったときに測り直す。
  useLayoutEffect(() => {
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(document.body);
    return () => observer.disconnect();
  }, [beforeEdge, measure]);
  useEffect(() => {
    opened.current = true;
  }, [opened]);
  const range = useMemberRange(shown, currentIndex);
  const virtualizer = useWindowVirtualizer({
    count: shown.length,
    estimateSize: () => memberRowHeight,
    gap: memberRowGap,
    overscan: memberOverscan,
    scrollMargin: margin,
    scrollPaddingStart: paddingStart,
    scrollPaddingEnd: memberScrollMargin,
    rangeExtractor: range.rangeExtractor,
    getItemKey: range.getItemKey,
  });
  const reveal = useCallback(
    () => revealMember(virtualizer, currentIndex),
    [virtualizer, currentIndex],
  );
  useEffect(() => {
    jump.current = reveal;
  }, [jump, reveal]);
  return (
    <MemberRows
      {...props}
      listRef={listRef}
      items={virtualizer.getVirtualItems()}
      totalSize={virtualizer.getTotalSize()}
      scrollMargin={virtualizer.options.scrollMargin}
      measure={virtualizer.measureElement}
      onFocus={range.onFocus}
      onBlur={range.onBlur}
    />
  );
}

/**
 * MemberRows はメンバーの並びのうち、描く行だけを並べる。`ul` は全件の高さを持ち
 * （後ろの端の目印はその下にある）、各行はその中の位置へ置く。
 */
function MemberRows({
  shown,
  currentIndex,
  first,
  total,
  backTo,
  listRef,
  items,
  totalSize,
  scrollMargin,
  measure,
  onFocus,
  onBlur,
}: MemberListProps & {
  listRef: Ref<HTMLUListElement>;
  items: VirtualItem[];
  totalSize: number;
  scrollMargin: number;
  measure: (element: Element | null) => void;
  onFocus: (event: FocusEvent<HTMLUListElement>) => void;
  onBlur: (event: FocusEvent<HTMLUListElement>) => void;
}) {
  return (
    <ul
      ref={listRef}
      className="relative"
      style={{ height: totalSize }}
      onFocus={onFocus}
      onBlur={onBlur}
    >
      {items.map((item) => {
        const member = shown[item.index]!;
        const row: MemberRow = {
          index: item.index,
          measure,
          offset: item.start - scrollMargin,
          position: first + item.index + 1,
          total,
        };
        return item.index === currentIndex ? (
          <CurrentMember key={item.key} row={row} video={member} />
        ) : (
          <MemberItem key={item.key} row={row} video={member} backTo={backTo} />
        );
      })}
    </ul>
  );
}

/** MemberRow は描く1行の並びの中の位置である。 */
interface MemberRow {
  /** shown の中の位置。仮想化が行を測るときに読む。 */
  index: number;
  measure: (element: Element | null) => void;
  /** 並びの上端からの位置（px）。 */
  offset: number;
  /** グループ全体の中の番号（1 始まり）。 */
  position: number;
  total: number;
}

/** memberRowProps は描く行の `li` の属性である。描かない行があっても何本目かを読み上げる。 */
function memberRowProps(row: MemberRow) {
  return {
    ref: row.measure,
    "data-index": row.index,
    "aria-posinset": row.position,
    "aria-setsize": row.total,
    style: { transform: `translateY(${String(row.offset)}px)` },
  };
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
      {failed && <p className="text-sm text-fg-muted">{t.player.related.moreFailed}</p>}
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
  row,
  video,
  backTo,
}: {
  row: MemberRow;
  video: Video;
  backTo: string;
}) {
  const preview = useHoverPreview(video);
  const { scrub, release, setLink } = useRelatedScrub(video, preview);
  return (
    <li {...memberRowProps(row)} className="absolute inset-x-0 top-0">
      <Link
        ref={setLink}
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={memberLinkLabel(video)}
        onPointerEnter={preview.onPointerEnter}
        onPointerLeave={release}
        className="-m-1.5 flex gap-3 rounded-lg p-1.5 transition-colors hover:bg-hover-wash"
      >
        <MemberNumber position={row.position} />
        <VideoThumbnail video={video} className="w-40" preview={preview} scrub={scrub} />
        <MemberTitle video={video} />
      </Link>
    </li>
  );
}

/** CurrentMember は今見ているメンバーの行である。押せないので、リンクにせず hover でも変えない。 */
function CurrentMember({ row, video }: { row: MemberRow; video: Video }) {
  const watched = video.progress?.completed === true;
  return (
    <li {...memberRowProps(row)} aria-current="true" className="absolute inset-x-0 top-0">
      {/* 左の線の太さの分だけ左の余白を減らし、番号とサムネイルの位置を他の行とそろえる。 */}
      <div className="-m-1.5 flex gap-3 rounded-lg border-l-2 border-accent bg-active-wash p-1.5 pl-1">
        <MemberNumber position={row.position} />
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
