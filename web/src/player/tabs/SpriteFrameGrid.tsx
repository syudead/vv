import { LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef } from "react";

import type { SeekThumbnailSprite } from "../../api/client";
import { t } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { seekSpriteCell } from "../../lib/seekSprite";
import { ScrubFrame } from "../../ui/ScrubPreview";
import { VideoThumbnail } from "../../ui/VideoThumbnail";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { ReadFailure } from "./panelParts";
import { type SpriteSheets, useSpriteSheets } from "./useSpriteFrame";

export interface SpriteFrameGridProps {
  spriteUrl: string;
  /** The frame whose position is being sent, if any. */
  sendingFrame: number | null;
  /** Some thumbnail request is running, so a press does nothing. */
  locked: boolean;
  /** The position of the current thumbnail when it is a frame, to mark that frame. */
  currentMs: number | undefined;
  onPick: (frame: number, positionMs: number) => void;
}

/** Read a frame's sheet once the frame comes near the viewport; at once without an observer. */
function useSheetObserver(requestSheet: (index: number) => void) {
  const observer = useRef<IntersectionObserver | null>(null);
  useEffect(() => () => observer.current?.disconnect(), []);
  return useCallback(
    (element: HTMLElement | null) => {
      if (element === null) return;
      const sheet = Number(element.dataset.sheet);
      if (typeof IntersectionObserver === "undefined") {
        requestSheet(sheet);
        return;
      }
      observer.current ??= new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (!entry.isIntersecting) continue;
            observer.current?.unobserve(entry.target);
            requestSheet(Number((entry.target as HTMLElement).dataset.sheet));
          }
        },
        { rootMargin: "50%" },
      );
      observer.current.observe(element);
    },
    [requestSheet],
  );
}

/**
 * Every frame of the seek sprite as a small thumbnail; pressing one makes that position the
 * thumbnail. Sheets are read as their frames come into view.
 */
export default function SpriteFrameGrid(props: SpriteFrameGridProps) {
  const sheets = useSpriteSheets(props.spriteUrl, true);
  const text = t.player.tabs;
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-3">
      <div className="flex flex-col">
        <span className="text-sm text-foreground">{text.frames}</span>
        <span className="text-2xs text-muted-foreground">{text.framesNote}</span>
      </div>
      {sheets.failed ? (
        <ReadFailure text={text.framesFailed} onRetry={sheets.retry} />
      ) : sheets.sprite === null ? (
        <div
          role="status"
          aria-label={text.framesLoading}
          className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-5"
        >
          <Skeleton className="aspect-video" />
          <Skeleton className="aspect-video" />
          <Skeleton className="aspect-video" />
        </div>
      ) : (
        <Frames {...props} sheets={sheets} sprite={sheets.sprite} />
      )}
    </div>
  );
}

function Frames({
  sheets,
  sprite,
  sendingFrame,
  locked,
  currentMs,
  onPick,
}: SpriteFrameGridProps & { sheets: SpriteSheets; sprite: SeekThumbnailSprite }) {
  const observe = useSheetObserver(sheets.requestSheet);
  const current =
    currentMs === undefined ? undefined : seekSpriteCell(sprite, currentMs).frame;
  const frames = Array.from({ length: sprite.frameCount }, (_, frame) => frame);
  return (
    <>
      <ul
        aria-label={t.player.tabs.frames}
        className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-5"
      >
        {frames.map((frame) => {
          const positionMs = frame * sprite.intervalMs;
          const cell = seekSpriteCell(sprite, positionMs);
          const time = formatDuration(positionMs);
          const sending = sendingFrame === frame;
          return (
            <li key={frame} ref={observe} data-sheet={cell.sheet}>
              <Button
                variant="ghost"
                aria-label={t.player.tabs.useFrameAt(time)}
                aria-disabled={(locked && !sending) || undefined}
                aria-busy={sending || undefined}
                onClick={() => {
                  if (!locked) onPick(frame, positionMs);
                }}
                className="h-auto w-full flex-col items-stretch gap-1 p-1"
              >
                <VideoThumbnail selected={frame === current}>
                  <ScrubFrame frame={sheets.frame(cell)} />
                  {sending && (
                    <span className="absolute inset-0 flex items-center justify-center bg-overlay">
                      <LoaderCircle
                        aria-hidden="true"
                        className="animate-spin motion-reduce:animate-none"
                      />
                    </span>
                  )}
                </VideoThumbnail>
                <span className="text-left font-mono text-2xs text-muted-foreground tabular-nums">
                  {time}
                </span>
              </Button>
            </li>
          );
        })}
      </ul>
      {sheets.sheetFailed && (
        <ReadFailure text={t.player.tabs.framesFailed} onRetry={sheets.retry} />
      )}
    </>
  );
}
