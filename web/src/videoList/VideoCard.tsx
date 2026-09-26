import { AlertTriangle, Check, Folder, Globe } from "lucide-react";
import { memo, type MouseEvent, type ReactNode } from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { useAudience } from "../auth/audience";
import { cn } from "../lib/cn";
import {
  formatBytes,
  formatDuration,
  formatRelative,
  qualityLabel,
  unplayableText,
  watchState,
  isNarrowVideo,
  watchedRatio,
} from "../lib/format";
import Checkbox from "../ui/Checkbox";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
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
   * 題名の下に呼び出し側の行を足す口（Plan の Structural Decisions 10）。省くと
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

/** PublicMark は公開の印（地球のアイコンと、読み上げ用の「公開」）である。 */
function PublicMark({ className }: { className?: string }) {
  return (
    <>
      <Globe aria-hidden="true" className={cn("size-3 shrink-0 text-fg", className)} />
      <span className="sr-only">公開</span>
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
        label={`「${video.title}」を選択`}
        className={previewing ? "!bg-navbar" : undefined}
        onClick={(event: MouseEvent) => event.stopPropagation()}
      />
    </div>
  );
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
  const showTagsRow = tagsRow !== undefined && video.tags.length > 0;
  const preview = useCardPreview({
    video,
    selectionMode,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart,
  });
  const { showingPreview, releasePreview, startPreview } = preview;

  return (
    <article
      data-video-id={video.id}
      onPointerEnter={startPreview}
      onPointerLeave={releasePreview}
      className={cn(
        "group relative flex flex-col overflow-hidden rounded-lg bg-surface shadow-card transition-[box-shadow,transform] duration-200 ease-out-quart",
        // リンクの輪郭は overflow-hidden で切れるので、キーボードフォーカスは箱の外側に出す。
        "has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-link",
        "hover:-translate-y-0.5",
        // 装飾的な動きは動きを減らす設定で止める（library-ui.md 4）。影の最終状態は残す。
        "motion-reduce:transition-none motion-reduce:hover:translate-y-0",
        "hover:shadow-card-hover",
        selected && "ring-2 ring-accent",
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
          onPreviewCancel={releasePreview}
        />
      )}

      <Link
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={
          location === undefined ? video.title : `${video.title}、${location.label}`
        }
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode && onSelect !== undefined) {
            event.preventDefault();
            onSelect(video.id, !selected);
          }
        }}
        className="flex min-w-0 flex-col outline-none"
      >
        <div className="relative aspect-video w-full overflow-hidden bg-navbar">
          <CardMedia video={video} preview={preview} />

          {(publicMark || duration !== "") && (
            <span
              className={cn(
                "absolute right-2 bottom-2 flex items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-[11px] font-medium tabular-nums backdrop-blur-sm",
                showingPreview ? "bg-navbar text-fg" : "bg-navbar/85 text-fg",
              )}
            >
              {publicMark && <PublicMark />}
              {duration !== "" && <span>{duration}</span>}
            </span>
          )}

          {ratio !== null && (
            <span
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(ratio * 100)}
              aria-label="再生済みの割合"
              className={cn(
                "absolute inset-x-0 bottom-0 h-[5px]",
                showingPreview ? "bg-fg-subtle" : "bg-fg-subtle/50",
              )}
            >
              <span
                className="block h-full bg-accent"
                style={{ width: `${String(Math.round(ratio * 100))}%` }}
              />
            </span>
          )}

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
              "line-clamp-2 text-sm leading-5 font-medium break-all",
              state === "watched" ? "text-fg-muted" : "text-fg",
            )}
          >
            {video.title}
          </h3>
          {location !== undefined && (
            <p className="flex min-w-0 items-center gap-1 text-xs text-fg-muted">
              <Folder aria-hidden="true" className="size-3 shrink-0 text-fg-subtle" />
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

  return (
    <tr
      data-video-id={video.id}
      className={cn(
        "group relative transition-colors hover:bg-hover-wash",
        selected && "bg-accent-soft",
      )}
    >
      {/* 選択を持たない画面（ゲストの一覧）では、選択の列ごと描かない。 */}
      {onSelect !== undefined && (
        <td className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(video.id, next)}
            label={`「${video.title}」を選択`}
            className={cn(
              "transition-opacity",
              selectionMode || selected
                ? "opacity-100"
                : "opacity-40 group-hover:opacity-100",
            )}
          />
        </td>
      )}
      <td className="w-32 py-1.5 pr-2">
        <div className="relative aspect-video w-28 overflow-hidden rounded-sm bg-navbar">
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
            <span className="absolute inset-x-0 bottom-0 h-[3px] bg-fg-subtle/50">
              <span
                className="block h-full bg-accent"
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
            "line-clamp-2 text-sm font-medium break-all hover:text-link",
            state === "watched" ? "text-fg-muted" : "text-fg",
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
      <td className="hidden w-16 pr-4 text-right text-xs text-fg-muted tabular-nums sm:table-cell">
        {state === "watched" && (
          <Check className="ml-auto size-4 text-success" aria-label="視聴済み" />
        )}
      </td>
      <td className="w-20 pr-4 text-right text-sm text-fg tabular-nums">
        {/* 公開の印は時間の直前に置く（ui-design.md「Card」）。 */}
        <span className="inline-flex items-center justify-end gap-1.5">
          {publicMark && <PublicMark />}
          {duration}
        </span>
      </td>
      <td className="hidden w-20 pr-4 text-right text-sm font-bold text-fg uppercase md:table-cell">
        {quality}
      </td>
      <td className="hidden w-24 pr-4 text-right text-sm text-fg-muted tabular-nums md:table-cell">
        {formatBytes(video.sizeBytes)}
      </td>
      <td className="hidden w-28 pr-3 text-right text-sm text-fg-muted tabular-nums lg:table-cell">
        {formatRelative(video.addedAt)}
      </td>
    </tr>
  );
});
