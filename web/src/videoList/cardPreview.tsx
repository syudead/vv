import { ImageOff } from "lucide-react";
import {
  type PointerEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import type { Video } from "../api/client";
import { cn } from "../lib/cn";
import { isNarrowVideo, unplayableText } from "../lib/format";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";

/**
 * CardPreviewOptions は一覧のカードの hover プレビューの条件である。動画のカード
 * （VideoCard）とグループのカード（GroupCard、`cover` を流す）が同じ規則で使う
 * （400ms、同時に1件。specs/017-folder-groups/ui-design.md「Group card」）。
 */
export interface CardPreviewOptions {
  /** プレビューを流す動画（グループのカードでは `cover`）。 */
  video: Video;
  selectionMode: boolean;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
}

/** CardPreview は useCardPreview の結果である。 */
export interface CardPreview {
  attempting: boolean;
  previewActive: boolean;
  showingPreview: boolean;
  setVideoElement: (element: HTMLVideoElement | null) => void;
  setPlaying: (playing: boolean) => void;
  releasePreview: () => void;
  startPreview: (event: PointerEvent<HTMLElement>) => void;
}

/**
 * useCardPreview は、マウスを 400ms 載せたらプレビューを流し、離したり別のカードが
 * 流し始めたりしたら止める。
 */
export function useCardPreview({
  video,
  selectionMode,
  activePreviewId,
  previewResetEpoch = 0,
  onPreviewStart,
}: CardPreviewOptions): CardPreview {
  const eligible =
    unplayableText(video) === null &&
    video.previewState === "done" &&
    video.previewUrl !== undefined;
  const coordinated = activePreviewId !== undefined && onPreviewStart !== undefined;
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

  return {
    attempting,
    previewActive,
    showingPreview,
    setVideoElement,
    setPlaying,
    releasePreview,
    startPreview,
  };
}

/**
 * CardMedia はカードのサムネイルと、その上に重ねるプレビューの動画である。
 * サムネイルが無いときは代わりの表示を出す。
 */
export function CardMedia({ video, preview }: { video: Video; preview: CardPreview }) {
  const { attempting, previewActive, showingPreview, setVideoElement, setPlaying } =
    preview;
  return (
    <div
      data-preview-media="true"
      className="absolute inset-0 transition-transform duration-300 ease-out-quart group-hover:scale-[1.03] motion-reduce:transition-none motion-reduce:group-hover:scale-100"
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
          onError={preview.releasePreview}
          className={cn(
            "absolute inset-0 h-full w-full object-contain",
            showingPreview ? "opacity-100" : "opacity-0",
          )}
        />
      )}
    </div>
  );
}
