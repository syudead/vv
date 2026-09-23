import { AlertTriangle, Check, ImageOff } from "lucide-react";
import {
  memo,
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
  onSelect: (id: number, selected: boolean) => void;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
  onPreviewReset?: () => void;
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
}: Pick<VideoCardProps, "video" | "selected" | "selectionMode" | "onSelect"> & {
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
 * VideoCard は Stash の scene-card と同じ箱型。サムネイルはカードの端まで、
 * 右下に「720P 59:11」の文字（ホバーで消える）、下に題名と日付・大きさ。
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
  } = props;
  const eligible = video.previewState === "done" && video.previewUrl !== undefined;
  const {
    duration,
    unplayable: rawUnplayable,
    state,
    ratio,
    quality,
  } = useCardState(video);
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
        "hover:-translate-y-0.5",
        "hover:shadow-card-hover",
        selected && "ring-2 ring-accent",
        selectionMode && "select-none",
      )}
    >
      <SelectCheck
        video={video}
        selected={selected}
        selectionMode={selectionMode}
        onSelect={onSelect}
        previewing={showingPreview}
        onPreviewCancel={releasePreview}
      />

      <Link
        to={`/videos/${String(video.id)}`}
        state={{ from: backTo }}
        aria-label={video.title}
        onClick={(event) => {
          onPreviewReset?.();
          if (selectionMode) {
            event.preventDefault();
            onSelect(video.id, !selected);
          }
        }}
        className="flex min-w-0 flex-col outline-none"
      >
        <div className="relative aspect-video w-full overflow-hidden bg-navbar">
          <div
            data-preview-media="true"
            className="absolute inset-0 transition-transform duration-300 ease-out-quart group-hover:scale-[1.03]"
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

          {(quality !== "" || duration !== "") && (
            <span
              className={cn(
                "absolute right-2 bottom-2 flex items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-[11px] font-medium tabular-nums backdrop-blur-sm",
                showingPreview ? "bg-navbar text-fg" : "bg-navbar/85 text-fg",
              )}
            >
              {quality !== "" && <span className="text-accent">{quality}</span>}
              {duration !== "" && <span>{duration}</span>}
            </span>
          )}

          {state === "watched" && (
            <span
              className={cn(
                "absolute top-2 right-2 flex size-6 items-center justify-center text-success backdrop-blur-sm",
                showingPreview ? "rounded-full bg-navbar" : "rounded-full bg-navbar/85",
              )}
            >
              <Check className="size-3.5" strokeWidth={3} />
              <span className="sr-only">視聴済み</span>
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

        <div className="flex min-w-0 flex-col gap-1 px-3 pt-2 pb-3">
          <h3
            title={video.title}
            className={cn(
              "line-clamp-2 text-sm leading-5 font-medium break-all",
              state === "watched" ? "text-fg-muted" : "text-fg",
            )}
          >
            {video.title}
          </h3>
          <p className="flex items-center gap-2 text-xs text-fg-muted tabular-nums">
            <span>{formatRelative(video.addedAt)}</span>
            <span className="text-fg-subtle">·</span>
            <span>{formatBytes(video.sizeBytes)}</span>
            {video.videoCodec !== undefined && (
              <>
                <span className="text-fg-subtle">·</span>
                <span className="uppercase">{video.videoCodec}</span>
              </>
            )}
          </p>
        </div>
      </Link>
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
              onSelect(video.id, !selected);
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
