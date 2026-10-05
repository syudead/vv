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
import {
  cardFrameClass,
  cardLinkClass,
  cardThumbnailClass,
} from "../ui/patterns/card-grid";
import { CardMedia, useCardPreview } from "./cardPreview";

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
function PublicMark() {
  return (
    <>
      <Globe aria-hidden="true" className="size-3 shrink-0" />
      <span className="sr-only">{t.list.card.public}</span>
    </>
  );
}

/**
 * SelectCheck はカードの左上の選択のチェックである。リンクの外に置き、ポインタが入ったら
 * ホバープレビューを止める。選択中か選択モードの間は常に、それ以外はカードの hover と
 * フォーカス、`hover:none` の端末で見せる。
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
  const { startPreview } = preview;
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
      className={cn(cardFrameClass(selected), selectionMode && "select-none")}
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
        className={cardLinkClass}
      >
        <VideoThumbnail className={cardThumbnailClass}>
          <CardMedia video={video} preview={preview} scrubFrame={scrub.frame} />

          {(publicMark || duration !== "") && (
            <VideoThumbnailDuration>
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

          {/* 帯は時刻の表示とバーより前、全面の警告と選択のチェックより後ろ（R-5）。 */}
          <ScrubBand scrub={scrub} />

          {rawUnplayable !== null && (
            <VideoThumbnailNotice>
              <AlertTriangle aria-hidden="true" />
              <span>{rawUnplayable}</span>
            </VideoThumbnailNotice>
          )}
        </VideoThumbnail>

        <div
          className={cn("flex min-w-0 flex-col gap-1 px-3 pt-2", !showTagsRow && "pb-3")}
        >
          <h3
            title={video.title}
            className={cn(
              "line-clamp-2 text-sm font-semibold break-all sm:text-base",
              state === "watched" ? "text-muted-foreground" : "text-foreground",
            )}
          >
            {video.title}
          </h3>
          {location !== undefined && (
            <p className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
              <Folder aria-hidden="true" className="size-3 shrink-0" />
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
      {/* 付け外しはサムネイルの右上、リンクの外に置く（チェックと同じ。DOM の順は
          チェック → リンク → 付け外し → タグの行。ui-design.md「Placement」）。 */}
      {owner && (
        // ポインタが入ったらホバープレビューを止め、ここからはプレビューを始めない。
        <VideoThumbnailMark data-preview-checkbox="true" onPointerEnter={release}>
          <VideoFavorite video={video} variant="card" />
        </VideoThumbnailMark>
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
    <TableRow
      data-video-id={video.id}
      data-state={selected ? "selected" : undefined}
      className="group"
    >
      {/* 選択を持たない画面（ゲストの一覧）では、選択の列ごと描かない。 */}
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
          to={`/videos/${String(video.id)}`}
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
      </TableCell>
      {/* お気に入りは題名の列の直後の列（ui-design.md「List view row」）。ゲストには列ごと描かない。 */}
      {owner && (
        <TableCell className="w-8 px-0">
          <VideoFavorite video={video} variant="row" />
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
        {/* 公開の印は時間の直前に置く（ui-design.md「Card」）。 */}
        <span className="inline-flex items-center justify-end gap-1.5">
          {publicMark && <PublicMark />}
          {duration}
        </span>
      </TableCell>
      <TableCell className="hidden w-list-number pr-4 text-right font-semibold uppercase md:table-cell">
        {quality}
      </TableCell>
      <TableCell className="hidden w-list-number-wide pr-4 text-right text-muted-foreground tabular-nums md:table-cell">
        {formatBytes(video.sizeBytes)}
      </TableCell>
      <TableCell className="hidden w-list-date pr-3 text-right text-muted-foreground tabular-nums lg:table-cell">
        {formatRelative(video.addedAt)}
      </TableCell>
    </TableRow>
  );
});
