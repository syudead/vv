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
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { isNarrowVideo, unplayableText } from "../lib/format";
import { ScrubFrame, type ScrubFrameData } from "../ui/ScrubPreview";
import ThumbnailBackdrop from "../ui/ThumbnailBackdrop";
import {
  type PreviewCoordinator,
  usePreviewActive,
  usePreviewChanges,
} from "./usePreviewCoordination";

/** Coordinate one preview after 400ms across video cards and FolderArt. */
export interface CardPreviewOptions {
  video: Video;
  selectionMode: boolean;
  previewCoordinator?: PreviewCoordinator;
}

export interface CardPreview {
  attempting: boolean;
  previewActive: boolean;
  showingPreview: boolean;
  setVideoElement: (element: HTMLVideoElement | null) => void;
  setPlaying: (playing: boolean) => void;
  releasePreview: () => void;
  startPreview: (event: PointerEvent<HTMLElement>) => void;
  /** Stop the delay and pause media without discarding its source or element. */
  suspendPreview: () => void;
  /** Restart the 400ms delay and resume from the paused frame. */
  resumePreview: () => void;
}

export function useCardPreview({
  video,
  selectionMode,
  previewCoordinator,
}: CardPreviewOptions): CardPreview {
  const eligible =
    unplayableText(video) === null &&
    video.previewState === "done" &&
    video.previewUrl !== undefined;
  const previewActive = usePreviewActive(previewCoordinator, video.id);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const lifecycle = useRef(0);
  const suspended = useRef(false);
  const waiting = useRef(false);
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
    suspended.current = false;
    waiting.current = false;
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
    if (selectionMode || previewCoordinator?.state().activeId !== video.id) {
      releasePreview();
    }
  }, [previewCoordinator, releasePreview, selectionMode, video.id]);
  usePreviewChanges(previewCoordinator, (current, previous) => {
    if (current.epoch !== previous.epoch || current.activeId !== video.id)
      releasePreview();
  });

  // Layout cleanup runs before React detaches videoRef, so navigation/unmount
  // can still pause the element and release its resource.
  useLayoutEffect(() => () => releasePreview(), [releasePreview]);

  // Release on play failure only if no later release or suspension superseded it.
  // Do not release when pause aborts a pending play during scrubbing.
  const playElement = useCallback(
    (element: HTMLVideoElement) => {
      const currentLifecycle = lifecycle.current;
      try {
        const result = element.play();
        result?.catch(() => {
          if (lifecycle.current === currentLifecycle) releasePreview();
        });
      } catch {
        releasePreview();
      }
    },
    [releasePreview],
  );

  useEffect(() => {
    if (!attempting || suspended.current) return;
    const element = videoRef.current;
    if (element === null) return;
    playElement(element);
  }, [attempting, playElement]);

  const scheduleStart = useCallback(() => {
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      timer.current = null;
      previewCoordinator?.start(video.id);
      setAttempting(true);
    }, 400);
  }, [previewCoordinator, video.id]);

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
      scheduleStart();
    },
    [eligible, releasePreview, scheduleStart, selectionMode],
  );

  const suspendPreview = useCallback(() => {
    if (suspended.current) return;
    suspended.current = true;
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
      waiting.current = true;
    }
    const element = videoRef.current;
    if (element !== null) {
      // Advance generation so pause-induced play rejection cannot release the preview.
      lifecycle.current += 1;
      element.pause();
    }
  }, []);

  const resumePreview = useCallback(() => {
    if (!suspended.current) return;
    suspended.current = false;
    if (waiting.current) {
      waiting.current = false;
      scheduleStart();
      return;
    }
    const element = videoRef.current;
    if (element !== null) playElement(element);
  }, [playElement, scheduleStart]);

  return {
    attempting,
    previewActive,
    showingPreview,
    setVideoElement,
    setPlaying,
    releasePreview,
    startPreview,
    suspendPreview,
    resumePreview,
  };
}

export function CardMedia({
  video,
  preview,
  scrubFrame = null,
}: {
  video: Video;
  preview: CardPreview;
  scrubFrame?: ScrubFrameData | null;
}) {
  const { attempting, previewActive, showingPreview, setVideoElement, setPlaying } =
    preview;
  return (
    <div
      data-preview-media="true"
      className="absolute inset-0 transition-transform duration-300 ease-out-quart group-hover:scale-105 motion-reduce:transition-none motion-reduce:group-hover:scale-100"
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
        <div className="flex h-full w-full flex-col items-center justify-center gap-1.5 text-muted-foreground">
          <ImageOff className="size-6" strokeWidth={1.5} />
          <span className="text-xs">
            {video.thumbnailState === "failed"
              ? t.list.card.noImage
              : t.list.card.preparing}
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

      <ScrubFrame frame={scrubFrame} />
    </div>
  );
}
