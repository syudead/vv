import { AlertTriangle, Check, Folder, ImageOff } from "lucide-react";
import {
  memo,
  type ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent,
  type PointerEvent,
} from "react";
import { Link } from "react-router";

import type { Video } from "../api/client";
import { cn } from "../lib/cn";
import {
  formatBytes,
  formatDuration,
  formatRelative,
  qualityLabel,
  unplayableText,
  watchState,
  watchedRatio,
} from "../lib/format";
import Checkbox from "../ui/Checkbox";

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
    activePreviewId = null,
    previewResetEpoch = 0,
    onPreviewStart,
    onPreviewReset,
    location,
    tagsRow,
  } = props;
  const { duration, unplayable: rawUnplayable, state, ratio } = useCardState(video);
  // タグが無い動画は行を出さない（ui-design.md「Tag row」）が、題名の下の余白は
  // 今の pb-3 のまま保つ（タグの有無で高さの余白が変わって見えないように）。
  const tagsRowNode = tagsRow?.(video);
  const showTagsRow = tagsRow !== undefined && video.tags.length > 0;
  const eligible =
    rawUnplayable === null &&
    video.previewState === "done" &&
    video.previewUrl !== undefined;
  const coordinated = props.activePreviewId !== undefined && onPreviewStart !== undefined;
  const previewActive = !coordinated || activePreviewId === video.id;
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const lifecycle = useRef(0);
  const observedResetEpoch = useRef(previewResetEpoch);
  const [attempting, setAttempting] = useState(false);
  const [playing, setPlaying] = useState(false);
  const showingPreview = playing && previewActive;

  const setVideoElement = useCallback((element: HTMLVideoElement | null) => {
    const previous = videoRef.current;
    videoRef.current = element;
    if (element === null && previous !== null) {
      queueMicrotask(() => {
        if (videoRef.current === previous || !previous.hasAttribute("src")) return;
        previous.pause();
        previous.removeAttribute("src");
        previous.load();
      });
    }
  }, []);

  const releasePreview = useCallback(() => {
    lifecycle.current += 1;
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    const element = videoRef.current;
    if (element !== null) {
      element.pause();
      element.removeAttribute("src");
      element.load();
    }
    setAttempting(false);
    setPlaying(false);
  }, []);

  useEffect(() => {
    const resetChanged = observedResetEpoch.current !== previewResetEpoch;
    observedResetEpoch.current = previewResetEpoch;
    if (resetChanged || selectionMode || activePreviewId !== video.id) releasePreview();
  }, [activePreviewId, previewResetEpoch, releasePreview, selectionMode, video.id]);

  // Layout cleanup runs before React detaches videoRef, so navigation/unmount
  // can still pause the element and release its resource.
  useLayoutEffect(() => () => releasePreview(), [releasePreview]);

  useEffect(() => {
    if (!attempting) return;
    const element = videoRef.current;
    if (element === null) return;
    const currentLifecycle = lifecycle.current;
    try {
      const result = element.play();
      result?.catch(() => {
        if (lifecycle.current === currentLifecycle) releasePreview();
      });
    } catch {
      releasePreview();
    }
  }, [attempting, releasePreview]);

  const startPreview = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const target = event.target;
      if (
        !eligible ||
        selectionMode ||
        event.pointerType !== "mouse" ||
        (target instanceof Element && target.closest("[data-preview-checkbox]"))
      ) {
        releasePreview();
        return;
      }
      if (timer.current !== null) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        timer.current = null;
        onPreviewStart?.(video.id);
        setAttempting(true);
      }, 400);
    },
    [eligible, onPreviewStart, releasePreview, selectionMode, video.id],
  );

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
          <div
            data-preview-media="true"
            className="absolute inset-0 transition-transform duration-300 ease-out-quart group-hover:scale-[1.03] motion-reduce:transition-none motion-reduce:group-hover:scale-100"
          >
            {video.thumbnailUrl !== undefined ? (
              <img
                src={video.thumbnailUrl}
                alt=""
                loading="lazy"
                decoding="async"
                className={cn(
                  "h-full w-full object-cover object-top",
                  showingPreview && "opacity-0",
                )}
              />
            ) : (
              <div className="flex h-full w-full flex-col items-center justify-center gap-1.5 text-fg-subtle">
                <ImageOff className="size-6" strokeWidth={1.5} />
                <span className="text-xs">
                  {video.thumbnailState === "failed" ? "画像なし" : "準備中"}
                </span>
              </div>
            )}

            {attempting && previewActive && video.previewUrl !== undefined && (
              <video
                ref={setVideoElement}
                src={video.previewUrl}
                muted
                playsInline
                loop
                preload="auto"
                controls={false}
                aria-hidden="true"
                tabIndex={-1}
                onPlaying={() => setPlaying(true)}
                onError={releasePreview}
                className={cn(
                  "absolute inset-0 h-full w-full object-cover object-top",
                  showingPreview ? "opacity-100" : "opacity-0",
                )}
              />
            )}
          </div>

          {duration !== "" && (
            <span
              className={cn(
                "absolute right-2 bottom-2 rounded-sm px-1.5 py-0.5 text-[11px] font-medium tabular-nums backdrop-blur-sm",
                showingPreview ? "bg-navbar text-fg" : "bg-navbar/85 text-fg",
              )}
            >
              {duration}
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

  return (
    <tr
      data-video-id={video.id}
      className={cn(
        "group relative transition-colors hover:bg-hover-wash",
        selected && "bg-accent-soft",
      )}
    >
      <td className="w-10 pl-3">
        <Checkbox
          checked={selected}
          onCheckedChange={(next) => onSelect?.(video.id, next)}
          label={`「${video.title}」を選択`}
          className={cn(
            "transition-opacity",
            selectionMode || selected
              ? "opacity-100"
              : "opacity-40 group-hover:opacity-100",
          )}
        />
      </td>
      <td className="w-32 py-1.5 pr-2">
        <div className="relative aspect-video w-28 overflow-hidden rounded-sm bg-navbar">
          {video.thumbnailUrl !== undefined && (
            <img
              src={video.thumbnailUrl}
              alt=""
              loading="lazy"
              decoding="async"
              className="h-full w-full object-cover object-top"
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
      <td className="w-20 pr-4 text-right text-sm text-fg tabular-nums">{duration}</td>
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
