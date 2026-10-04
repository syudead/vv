import { AlertTriangle, Check, Folder, Globe } from "lucide-react";
import {
  memo,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  useCallback,
  useEffect,
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
import Checkbox from "../ui/legacy/Checkbox";
import { ScrubBand, type ScrubPreview, useScrubPreview } from "../ui/ScrubPreview";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import { CardMedia, useCardPreview } from "./cardPreview";
import FavoriteToggle from "./FavoriteToggle";

export interface VideoCardProps {
  video: Video;
  /** 遷移元の一覧 URL。再生画面の「戻る」がここへ帰る。 */
  backTo: string;
  selected: boolean;
  selectionMode: boolean;
  /** 選択を持たない画面（フォルダ画面）では省き、チェックを描かない。 */
  onSelect?: (id: number, selected: boolean) => void;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
  /**
   * フォルダ画面の検索結果にだけ添える置き場所（ui-design.md「Search results」）。
   * `label` は表示・読み上げ名に使う文字列（先頭の側を省略して表示する）で、
   * `title` 属性には省略しない全体を入れる（最上位では登録フォルダの絶対パスから）。
   */
  location?: { label: string; title: string };
  /**
   * 題名の下に呼び出し側の行を足す口。省くと
   * 題名の下には何も足さない。ライブラリの格子表示とフォルダ画面はここへタグの行を
   * 渡す（specs/014-video-tags/ui-design.md「Tag row」）。タグの行はリンクの**外**、
   * 同じ `article` の中に置かれる（Structural Decisions 10、キーボードの入れ子を避ける）。
   *
   * `ReactNode` ではなく関数で受け取るのは、呼び出し側が安定した
   * 参照を渡せるようにするためである。`memo(VideoCard)` は props が前回と同じ
   * 参照なら再描画しない。ReactNode を直に渡すと、呼び出し側の描画のたびに
   * 新しい要素になり、無関係な状態変化でも全カードが作り直されてしまう（N4）。
   */
  tagsRow?: (video: Video) => ReactNode;
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

/**
 * usePublicMark は所有者の一覧で公開の印を出すかを返す
 * （specs/016-single-account-auth/ui-design.md「Visibility toggle」の「Card」）。
 * ゲストに見えるのはすべて公開の動画で、印に意味が無いので出さない。
 */
function usePublicMark(video: Video): boolean {
  return useAudience() === "owner" && video.public;
}

/**
 * VideoFavorite は動画のカード・行のお気に入りの付け外しである（所有者だけ。
 * specs/035-favorites/ui-design.md「Card」）。ゲストには描かない（「Guest degradation」）。
 */
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

/** PublicMark は公開の印（地球のアイコンと、読み上げ用の「公開」）である。 */
function PublicMark({ className }: { className?: string }) {
  return (
    <>
      <Globe
        aria-hidden="true"
        className={cn("size-3 shrink-0 text-foreground", className)}
      />
      <span className="sr-only">{t.list.card.public}</span>
    </>
  );
}

function SelectCheck({
  video,
  selected,
  selectionMode,
  onSelect,
  previewing,
  onPreviewCancel,
}: Pick<VideoCardProps, "video" | "selected" | "selectionMode"> & {
  onSelect: (id: number, selected: boolean) => void;
  previewing: boolean;
  onPreviewCancel: () => void;
}) {
  return (
    <div
      data-preview-checkbox="true"
      onPointerEnter={onPreviewCancel}
      className={cn(
        "absolute top-2 left-2 z-20 transition-opacity duration-150",
        selectionMode || selected
          ? "opacity-100"
          : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100",
      )}
    >
      <Checkbox
        checked={selected}
        onCheckedChange={(next) => onSelect(video.id, next)}
        label={t.list.card.select(video.title)}
        className={previewing ? "!bg-navbar" : undefined}
        onClick={(event: MouseEvent) => event.stopPropagation()}
      />
    </div>
  );
}

/**
 * useCardScrub はカードのスクラブの帯をループ再生（useCardPreview）とつなぐ
 * （specs/032-card-scrub-preview/research.md R-2、ui-design.md「Pointer rules」）。
 *
 * - 帯への出入りでループを一時停止・再開する。帯から下へカードの外に出たときは再開せず、
 *   カードの pointerleave の解放に任せる。
 * - 並び替えなどの reset（previewResetEpoch）、別のカードのプレビューの開始
 *   （activePreviewId）、解放（release）で、帯から出たのと同じに戻し取得を打ち切る。
 *   選択モードでは useScrubPreview が帯を外す。
 */
function useCardScrub({
  video,
  selectionMode,
  activePreviewId,
  previewResetEpoch,
  preview,
}: {
  video: Video;
  selectionMode: boolean;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
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

  const observed = useRef({ previewResetEpoch, activePreviewId });
  useEffect(() => {
    const previous = observed.current;
    observed.current = { previewResetEpoch, activePreviewId };
    const otherStarted =
      previous.activePreviewId !== activePreviewId &&
      activePreviewId !== undefined &&
      activePreviewId !== null &&
      activePreviewId !== video.id;
    if (previous.previewResetEpoch !== previewResetEpoch || otherStarted) leaveCard();
  }, [activePreviewId, leaveCard, previewResetEpoch, video.id]);

  const band: ScrubPreview = {
    ...scrub,
    bandHandlers: { ...scrub.bandHandlers, onPointerLeave: onBandLeave },
  };
  return { scrub: band, release, setArticle };
}

/**
 * VideoCard は箱型。サムネイルはカードの端まで、右下に再生時間、下に題名。
 */
function VideoCard(props: VideoCardProps) {
  const {
    video,
    backTo,
    selected,
    selectionMode,
    onSelect,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart,
    onPreviewReset,
    location,
    tagsRow,
  } = props;
  const { duration, unplayable: rawUnplayable, state, ratio } = useCardState(video);
  // タグが無い動画は行を出さない（ui-design.md「Tag row」）が、題名の下の余白は
  // 今の pb-3 のまま保つ（タグの有無で高さの余白が変わって見えないように）。
  const tagsRowNode = tagsRow?.(video);
  const publicMark = usePublicMark(video);
  const owner = useAudience() === "owner";
  const showTagsRow = tagsRow !== undefined && video.tags.length > 0;
  const preview = useCardPreview({
    video,
    selectionMode,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart,
  });
  const { showingPreview, startPreview } = preview;
  const { scrub, release, setArticle } = useCardScrub({
    video,
    selectionMode,
    activePreviewId,
    previewResetEpoch,
    preview,
  });
  // 帯にいる間のポインタの位置。時刻の表示とスクラブ位置のバーを差し替える
  // （ui-design.md「Time and bar」）。
  const scrubPosition = scrub.position;

  return (
    <article
      ref={setArticle}
      data-video-id={video.id}
      onPointerEnter={startPreview}
      onPointerLeave={release}
      className={cn(
        "group relative flex flex-col overflow-hidden rounded-md border border-border bg-card transition duration-200 ease-out-quart",
        // リンクの輪郭は overflow-hidden で切れるので、キーボードフォーカスは箱の外側に出す。
        "has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-ring has-[button:focus-visible]:outline-2 has-[button:focus-visible]:outline-offset-2 has-[button:focus-visible]:outline-ring",
        // 装飾的な動きは動きを減らす設定で止める（library-ui.md 4）。影の最終状態は残す。
        "motion-reduce:transition-none",
        "hover:border-input hover:shadow-card-hover",
        selected && "border-primary ring-2 ring-primary",
        selectionMode && "select-none",
      )}
    >
      {onSelect !== undefined && (
        <SelectCheck
          video={video}
          selected={selected}
          selectionMode={selectionMode}
          onSelect={onSelect}
          previewing={showingPreview}
          onPreviewCancel={release}
        />
      )}

      {/* リンクは flex-1 で伸ばし、同じ格子の行で高いカードとの差を引き受ける。タグの行を
          カードの下端へそろえつつ、その間の余白も押せる（開く・選択する）範囲に含めるためである。 */}
      <Link
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={
          location === undefined
            ? video.title
            : t.list.card.withLocation(video.title, location.label)
        }
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(video.id, !selected);
          }
        }}
        className="flex min-w-0 flex-1 flex-col outline-none"
      >
        <div className="relative aspect-video w-full overflow-hidden bg-navbar">
          <CardMedia video={video} preview={preview} scrubFrame={scrub.frame} />

          {(publicMark || duration !== "") && (
            <span className="absolute right-2 bottom-2 flex items-center gap-1.5 rounded-sm bg-overlay px-1.5 py-0.5 text-2xs font-medium text-foreground tabular-nums">
              {publicMark && <PublicMark />}
              {duration !== "" && (
                <span>
                  {scrubPosition === null
                    ? duration
                    : t.list.card.scrubTime(
                        formatDuration(scrubPosition.positionMs),
                        duration,
                      )}
                </span>
              )}
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
                "absolute inset-x-0 bottom-0 h-1 bg-overlay",
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
              className="pointer-events-none absolute inset-x-0 bottom-0 h-1 bg-overlay"
            >
              <span
                className="block h-full bg-foreground"
                style={{ width: `${String(scrubPosition.ratio * 100)}%` }}
              />
            </span>
          )}

          {/* 帯は時刻の表示とバーより前、全面の警告と選択のチェックより後ろ（R-5）。 */}
          <ScrubBand scrub={scrub} />

          {rawUnplayable !== null && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 bg-overlay text-warning">
              <AlertTriangle className="size-5" />
              <span className="px-3 text-center text-xs font-medium">
                {rawUnplayable}
              </span>
            </div>
          )}
        </div>

        <div
          className={cn("flex min-w-0 flex-col gap-1 px-3 pt-2", !showTagsRow && "pb-3")}
        >
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
              <Folder
                aria-hidden="true"
                className="size-3 shrink-0 text-muted-foreground"
              />
              {/* 先頭の側を省略し、末尾のフォルダ名を残す（011 のフォルダカードと同じ扱い）。
                  title 属性は省略しない全体（最上位では登録フォルダの絶対パスから）。 */}
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
      {owner && (
        // お気に入りはサムネイルの右上、リンクの外に置く（チェックと同じ。DOM の順は
        // チェック → リンク → 付け外し → タグの行。ui-design.md「Placement」）。ポインタが
        // 入ったらホバープレビューを止め、ここからはプレビューを始めない。
        <div
          data-preview-checkbox="true"
          onPointerEnter={release}
          className="absolute top-1.5 right-1.5 z-20 flex"
        >
          <VideoFavorite video={video} variant="card" />
        </div>
      )}
      {showTagsRow && (
        // タグの行はリンクの外（別の要素）に置くので、題名の下との間隔を今の
        // gap-1（4px）と同じに保つには、ここで pt-1 を明示する必要がある（B3）。
        <div className="flex min-w-0 flex-col gap-1 px-3 pt-1 pb-3">{tagsRowNode}</div>
      )}
    </article>
  );
}

export default memo(VideoCard);

/** VideoRow はリスト表示の 1 行。 */
export const VideoRow = memo(function VideoRow(props: VideoCardProps) {
  const { video, backTo, selected, selectionMode, onSelect } = props;
  const { duration, unplayable, state, ratio, quality } = useCardState(video);
  const publicMark = usePublicMark(video);
  const owner = useAudience() === "owner";

  return (
    <tr
      data-video-id={video.id}
      className={cn(
        "group relative transition-colors hover:bg-accent [&>td]:border-b [&>td]:border-border has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-ring has-[button:focus-visible]:outline-2 has-[button:focus-visible]:outline-ring",
        selected && "bg-primary-soft",
      )}
    >
      {/* 選択を持たない画面（ゲストの一覧）では、選択の列ごと描かない。 */}
      {onSelect !== undefined && (
        <td className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(video.id, next)}
            label={t.list.card.select(video.title)}
            className={cn(
              "transition-opacity",
              selectionMode || selected
                ? "opacity-100"
                : "opacity-40 group-hover:opacity-100",
            )}
          />
        </td>
      )}
      <td className="w-list-thumb-cell py-1.5 pr-2">
        <div className="relative aspect-video w-list-thumb overflow-hidden rounded-sm bg-navbar">
          {video.thumbnailUrl !== undefined && isNarrowVideo(video) && (
            <ThumbnailBackdrop src={video.thumbnailUrl} />
          )}
          {video.thumbnailUrl !== undefined && (
            <img
              src={video.thumbnailUrl}
              alt=""
              loading="lazy"
              decoding="async"
              className="relative h-full w-full object-contain"
            />
          )}
          {ratio !== null && (
            <span className="absolute inset-x-0 bottom-0 h-1 bg-overlay">
              <span
                className="block h-full bg-primary"
                style={{ width: `${String(Math.round(ratio * 100))}%` }}
              />
            </span>
          )}
        </div>
      </td>
      <td className="min-w-0 py-1.5 pr-4">
        <Link
          to={`/videos/${String(video.id)}`}
          state={{ from: backTo }}
          onClick={(event) => {
            if (selectionMode) {
              event.preventDefault();
              onSelect?.(video.id, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 text-sm font-medium break-all hover:text-primary",
            state === "watched" ? "text-muted-foreground" : "text-foreground",
          )}
        >
          {video.title}
        </Link>
        {unplayable !== null && (
          <span className="mt-0.5 flex items-center gap-1 text-xs text-warning">
            <AlertTriangle className="size-3" />
            {unplayable}
          </span>
        )}
      </td>
      {/* お気に入りは題名の列の直後の列（ui-design.md「List view row」）。ゲストには列ごと描かない。 */}
      {owner && (
        <td className="w-8">
          <VideoFavorite video={video} variant="row" />
        </td>
      )}
      <td className="hidden w-16 pr-4 text-right text-xs text-muted-foreground tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check
            className="ml-auto size-4 text-success"
            aria-label={t.list.card.watched}
          />
        )}
      </td>
      <td className="w-list-number pr-4 text-right text-sm text-foreground tabular-nums">
        {/* 公開の印は時間の直前に置く（ui-design.md「Card」）。 */}
        <span className="inline-flex items-center justify-end gap-1.5">
          {publicMark && <PublicMark />}
          {duration}
        </span>
      </td>
      <td className="hidden w-list-number pr-4 text-right text-sm font-semibold text-foreground uppercase md:table-cell">
        {quality}
      </td>
      <td className="hidden w-list-number-wide pr-4 text-right text-sm text-muted-foreground tabular-nums md:table-cell">
        {formatBytes(video.sizeBytes)}
      </td>
      <td className="hidden w-list-date pr-3 text-right text-sm text-muted-foreground tabular-nums lg:table-cell">
        {formatRelative(video.addedAt)}
      </td>
    </tr>
  );
});
