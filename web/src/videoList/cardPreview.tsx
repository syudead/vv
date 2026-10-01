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

/**
 * CardPreviewOptions は一覧の動画のカード（VideoCard）の hover プレビューの条件である
 * （400ms、同時に1件）。グループのカードはフォルダの絵柄（FolderArt）が前に出した1枚を
 * 流し、同じ usePreviewCoordination の調整に加わる（specs/017-folder-groups/ui-design.md
 * 「Group card」）。
 */
export interface CardPreviewOptions {
  /** プレビューを流す動画。 */
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
  /**
   * suspendPreview はスクラブの帯に入ったときに、400ms の待ちを止め、再生中・読み込み中の
   * 動画を src と要素を保ったまま止める（specs/032-card-scrub-preview/research.md R-2）。
   */
  suspendPreview: () => void;
  /**
   * resumePreview は帯からカードの中へ出たときに、待ちを 400ms から数え直し、止めた
   * 動画をその場面から再生し直す。
   */
  resumePreview: () => void;
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
  // 帯にいる間の一時停止。waiting は待ちの途中で止めたことを表す。
  const suspended = useRef(false);
  const waiting = useRef(false);
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
    const resetChanged = observedResetEpoch.current !== previewResetEpoch;
    observedResetEpoch.current = previewResetEpoch;
    if (resetChanged || selectionMode || activePreviewId !== video.id) releasePreview();
  }, [activePreviewId, previewResetEpoch, releasePreview, selectionMode, video.id]);

  // Layout cleanup runs before React detaches videoRef, so navigation/unmount
  // can still pause the element and release its resource.
  useLayoutEffect(() => () => releasePreview(), [releasePreview]);

  // play の失敗は、その後に解放・一時停止されていなければプレビューを解放する。
  // 一時停止の pause() が読み込み中の play() を打ち切った失敗では解放しない。
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
      onPreviewStart?.(video.id);
      setAttempting(true);
    }, 400);
  }, [onPreviewStart, video.id]);

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
      // 読み込み中の play() が pause() で打ち切られても解放しないよう、世代を進める。
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

/**
 * CardMedia はカードのサムネイルと、その上に重ねるプレビューの動画である。
 * サムネイルが無いときは代わりの表示を出す。スクラブの帯にいる間のコマ（scrubFrame）は
 * サムネイルとループの動画の前に、同じ media 層の中で重ねる
 * （specs/032-card-scrub-preview/ui-design.md「Frame」）。
 */
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
