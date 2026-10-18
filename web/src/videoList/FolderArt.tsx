import { EllipsisVertical, Play } from "lucide-react";
import {
  type PointerEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import type { FolderPreview } from "../api/client";
import { t } from "../i18n";
import { cn } from "../lib/cn";

/**
 * Use video IDs as stable keys, adding occurrence counts only for duplicate playlist covers. A
 * direct video and a group cover may reference the same video.
 */
function sheetKeys(previews: readonly FolderPreview[]): string[] {
  const seen = new Map<number, number>();
  return previews.map(({ videoId }) => {
    const count = seen.get(videoId) ?? 0;
    seen.set(videoId, count + 1);
    return count === 0 ? String(videoId) : `${String(videoId)}:${String(count)}`;
  });
}

const sheetLayout: Record<number, { left: number; top: number; rotate: number }[]> = {
  1: [{ left: 16, top: 14, rotate: 0 }],
  2: [
    { left: 5, top: 16, rotate: -3 },
    { left: 27, top: 12, rotate: 3 },
  ],
  3: [
    { left: 3, top: 16, rotate: -4 },
    { left: 16, top: 12, rotate: 0 },
    { left: 29, top: 16, rotate: 4 },
  ],
  4: [
    { left: 2, top: 16, rotate: -5 },
    { left: 11, top: 12, rotate: -1.5 },
    { left: 20, top: 16, rotate: 1.5 },
    { left: 30, top: 12, rotate: 5 },
  ],
  5: [
    { left: 0, top: 16, rotate: -6 },
    { left: 8, top: 12, rotate: -3 },
    { left: 16, top: 16, rotate: 0 },
    { left: 24, top: 12, rotate: 3 },
    { left: 32, top: 16, rotate: 6 },
  ],
};

const frontPlace = { left: 16, top: 6 };

const previewDelayMs = 400;

/**
 * Keep the thumbnail until muted preview playback starts; failures leave it intact. Notify
 * coordination on loading and abort on exit.
 */
function SheetPreview({ src, onStart }: { src: string; onStart?: () => void }) {
  const [armed, setArmed] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [failed, setFailed] = useState(false);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  // Start the delay once on promotion; onStart identity changes must not restart it.
  const onStartRef = useRef(onStart);
  useLayoutEffect(() => {
    onStartRef.current = onStart;
  });

  useEffect(() => {
    const timer = setTimeout(() => {
      onStartRef.current?.();
      setArmed(true);
    }, previewDelayMs);
    return () => clearTimeout(timer);
  }, []);

  const release = useCallback(() => {
    const element = videoRef.current;
    if (element === null) return;
    element.pause();
    element.removeAttribute("src");
    element.load();
  }, []);

  // Layout cleanup runs before ref detachment, allowing loading to stop here.
  useLayoutEffect(() => release, [release]);

  useEffect(() => {
    if (!armed || failed) return;
    const element = videoRef.current;
    if (element === null) return;
    // Release loading before removing the element because its ref then becomes unavailable.
    const fail = () => {
      release();
      setFailed(true);
    };
    try {
      element.play()?.catch(fail);
    } catch {
      fail();
    }
  }, [armed, failed, release]);

  if (!armed || failed) return null;
  return (
    <video
      ref={videoRef}
      src={src}
      data-folder-video=""
      muted
      playsInline
      loop
      preload="auto"
      controls={false}
      tabIndex={-1}
      onPlaying={() => setPlaying(true)}
      onError={() => {
        release();
        setFailed(true);
      }}
      className={cn(
        "absolute inset-0 h-full w-full bg-navbar object-contain",
        playing ? "opacity-100" : "opacity-0",
      )}
    />
  );
}

/**
 * Render within a relative 16:9 box and coordinate one preview across the list. Selection mode
 * disables promotion; another preview or reset returns the promoted sheet.
 */
export default function FolderArt({
  previews,
  totalCount = previews.length,
  onFrontChange,
  selectionMode = false,
  activePreviewId,
  previewResetEpoch,
  onPreviewStart,
}: {
  previews: readonly FolderPreview[];
  totalCount?: number;
  onFrontChange?: (preview: FolderPreview | null) => void;
  selectionMode?: boolean;
  activePreviewId?: number | null;
  previewResetEpoch?: number;
  onPreviewStart?: (id: number) => void;
}) {
  const shown = previews.slice(0, 4);
  const remaining = Math.max(0, totalCount - shown.length);
  const sheets = remaining > 0 ? [...shown, null] : shown;
  const keys = sheetKeys(shown);
  const layout = sheetLayout[sheets.length] ?? [];
  // A refreshed shorter cover list can leave the old position out of range.
  const [front, setFront] = useState<number | null>(null);
  const [playingId, setPlayingId] = useState<number | null>(null);
  const observedResetEpoch = useRef(previewResetEpoch);
  const observedActiveId = useRef(activePreviewId);

  const putBack = useCallback(() => {
    setFront(null);
    setPlayingId(null);
    onFrontChange?.(null);
  }, [onFrontChange]);

  useEffect(() => putBack(), [previews, putBack]);

  useEffect(() => {
    const resetChanged = observedResetEpoch.current !== previewResetEpoch;
    observedResetEpoch.current = previewResetEpoch;
    // Reset only for a different preview started elsewhere, not an old value before this card's own announcement arrives.
    const activeChanged = observedActiveId.current !== activePreviewId;
    observedActiveId.current = activePreviewId;
    const replaced =
      onPreviewStart !== undefined &&
      activeChanged &&
      playingId !== null &&
      activePreviewId !== playingId;
    if (resetChanged || selectionMode || replaced) putBack();
  }, [
    activePreviewId,
    onPreviewStart,
    playingId,
    previewResetEpoch,
    putBack,
    selectionMode,
  ]);

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (selectionMode || sheets.length === 0 || event.pointerType !== "mouse") {
      return;
    }
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = (event.clientX - rect.left) / rect.width;
    const next = Math.min(
      sheets.length - 1,
      Math.max(0, Math.floor(ratio * sheets.length)),
    );
    onFrontChange?.(sheets[next] ?? null);
    if (next === front) return;
    setFront(next);
    setPlayingId(null);
  };

  return (
    <div
      aria-hidden="true"
      data-folder-art=""
      onPointerMove={onPointerMove}
      onPointerLeave={putBack}
      className="absolute inset-x-3 top-3 bottom-2"
    >
      <div className="absolute top-0 left-0 h-3 w-2/5 rounded-t-md bg-popover" />
      <div className="absolute inset-x-0 top-2 bottom-0 overflow-hidden rounded-md rounded-tl-none bg-popover">
        {sheets.map((preview, index) => {
          const place = layout[index];
          if (place === undefined) return null;
          const isFront = front === index;
          return (
            <div
              key={preview === null ? "more" : keys[index]}
              data-folder-preview={preview === null ? undefined : ""}
              data-folder-more={preview === null ? "" : undefined}
              data-folder-front={isFront ? "" : undefined}
              className={cn(
                "absolute aspect-video overflow-hidden rounded-sm border border-input bg-navbar transition-all duration-200 ease-out-quart motion-reduce:transition-none",
                "w-folder-preview",
                isFront && "shadow-card-hover",
              )}
              // Use style for per-sheet positions and rotations; width comes from w-folder-preview.
              // Earlier sheets sit in front, with the promoted sheet above all.
              style={{
                zIndex: isFront ? sheets.length + 1 : sheets.length - index,
                left: `${String(isFront ? frontPlace.left : place.left)}%`,
                top: `${String(isFront ? frontPlace.top : place.top)}%`,
                transform: isFront ? "scale(1.12)" : `rotate(${String(place.rotate)}deg)`,
              }}
            >
              {preview === null ? (
                <div className="flex size-full flex-col items-center justify-center gap-1 bg-card text-sm font-medium text-card-foreground">
                  <span>{t.library.collection.more}</span>
                  <span className="text-xs text-muted-foreground">
                    {t.library.collection.remaining(remaining)}
                  </span>
                  {!isFront && (
                    <EllipsisVertical className="absolute top-1/2 right-1 size-4 -translate-y-1/2" />
                  )}
                </div>
              ) : (
                <img
                  src={preview.thumbnailUrl}
                  alt=""
                  loading="lazy"
                  decoding="async"
                  className="h-full w-full object-contain"
                />
              )}
              {isFront && preview !== null && preview.previewUrl !== undefined && (
                <SheetPreview
                  key={preview.videoId}
                  src={preview.previewUrl}
                  onStart={() => {
                    setPlayingId(preview.videoId);
                    onPreviewStart?.(preview.videoId);
                  }}
                />
              )}
              {isFront && preview !== null && onFrontChange !== undefined && (
                <span className="absolute bottom-1 left-1 flex max-w-full items-center gap-1 rounded-sm bg-overlay px-1.5 py-0.5 text-2xs text-foreground">
                  <Play className="size-3 shrink-0" />
                  {preview.title && <span className="truncate">{preview.title}</span>}
                </span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
