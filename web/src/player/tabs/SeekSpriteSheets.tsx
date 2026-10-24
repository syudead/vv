import { type CSSProperties, useCallback, useEffect, useRef, useState } from "react";

import type { SeekThumbnailSprite } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { formatDuration } from "../../lib/format";
import {
  seekSpriteCell,
  type SpriteCell,
  spriteCellBackground,
} from "../../lib/seekSprite";
import { Button } from "../../ui/shadcn/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../../ui/shadcn/dialog";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { PanelSection, ReadFailure } from "./panelParts";
import { type SpriteSheets, useSpriteSheets } from "./useSpriteFrame";

/** Read a sheet once it comes near the viewport; at once without an observer. */
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
 * The seek sprite's frames in time order, without the empty cells that pad a sheet; pressing one
 * shows it large in a dialog. A frame's sheet is read as the frame comes into view.
 */
export default function SeekSpriteSheets({ spriteUrl }: { spriteUrl: string }) {
  const sheets = useSpriteSheets(spriteUrl, true);
  const text = t.player.tabs;
  return (
    <PanelSection title={text.seekSprite}>
      {sheets.failed ? (
        <ReadFailure text={text.seekSpriteFailed} onRetry={sheets.retry} />
      ) : sheets.sprite === null ? (
        <Skeleton
          role="status"
          aria-label={text.seekSpriteLoading}
          className="aspect-video w-replace-preview"
        />
      ) : (
        <Frames sheets={sheets} sprite={sheets.sprite} />
      )}
    </PanelSection>
  );
}

function frameCell(sprite: SeekThumbnailSprite, frame: number): SpriteCell {
  return seekSpriteCell(sprite, frame * sprite.intervalMs);
}

/** One frame drawn from its loaded sheet, or the muted frame while the sheet is not loaded. */
function FrameImage({
  sheets,
  sprite,
  cell,
  label,
  className,
}: {
  sheets: SpriteSheets;
  sprite: SeekThumbnailSprite;
  cell: SpriteCell;
  label: UiText;
  className: string;
}) {
  const url = sheets.sheetUrl(cell.sheet);
  const style: CSSProperties = {
    aspectRatio: `${String(sprite.frameWidth)} / ${String(sprite.frameHeight)}`,
  };
  if (url !== null) {
    const background = spriteCellBackground(cell, sprite);
    Object.assign(style, {
      backgroundImage: `url("${url}")`,
      backgroundRepeat: "no-repeat",
      backgroundSize: background.backgroundSize,
      backgroundPosition: background.backgroundPosition,
    });
  }
  return (
    <span
      role="img"
      aria-label={label}
      className={cn("block bg-muted", className)}
      style={style}
    />
  );
}

function Frames({
  sheets,
  sprite,
}: {
  sheets: SpriteSheets;
  sprite: SeekThumbnailSprite;
}) {
  const observe = useSheetObserver(sheets.requestSheet);
  const [open, setOpen] = useState<number | null>(null);
  const text = t.player.tabs;
  const time = (frame: number) => formatDuration(frame * sprite.intervalMs);
  return (
    <>
      <ul
        aria-label={text.seekSprite}
        className="grid grid-cols-4 gap-1 sm:grid-cols-6 md:grid-cols-9"
      >
        {Array.from({ length: sprite.frameCount }, (_, frame) => {
          const cell = frameCell(sprite, frame);
          const loaded = sheets.sheetUrl(cell.sheet) !== null;
          return (
            <li key={frame} ref={observe} data-sheet={cell.sheet}>
              <Button
                variant="ghost"
                aria-label={text.openSeekSpriteFrame(time(frame))}
                title={time(frame)}
                disabled={!loaded}
                onClick={() => setOpen(frame)}
                className="block h-auto w-full overflow-hidden rounded-sm p-0 transition-opacity hover:opacity-80"
              >
                <FrameImage
                  sheets={sheets}
                  sprite={sprite}
                  cell={cell}
                  label={text.seekSpriteFrame(time(frame))}
                  className="w-full"
                />
              </Button>
            </li>
          );
        })}
      </ul>
      {sheets.sheetFailed && (
        <ReadFailure text={text.seekSpriteFailed} onRetry={sheets.retry} />
      )}
      <Dialog
        open={open !== null}
        onOpenChange={(next) => {
          if (!next) setOpen(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {open === null ? text.seekSprite : text.seekSpriteFrame(time(open))}
            </DialogTitle>
          </DialogHeader>
          {open !== null && (
            <FrameImage
              sheets={sheets}
              sprite={sprite}
              cell={frameCell(sprite, open)}
              label={text.seekSpriteFrame(time(open))}
              className="w-full rounded-md"
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
