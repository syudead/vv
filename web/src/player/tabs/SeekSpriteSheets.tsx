import { useCallback, useEffect, useRef, useState } from "react";

import type { SeekThumbnailSprite } from "../../api/client";
import { t } from "../../i18n";
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
 * The seek sprite's sheets as small previews; pressing one shows it large in a dialog. A sheet is
 * read as it comes into view.
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
        <Sheets sheets={sheets} sprite={sheets.sprite} />
      )}
    </PanelSection>
  );
}

function Sheets({
  sheets,
  sprite,
}: {
  sheets: SpriteSheets;
  sprite: SeekThumbnailSprite;
}) {
  const observe = useSheetObserver(sheets.requestSheet);
  const [open, setOpen] = useState<number | null>(null);
  const text = t.player.tabs;
  const count = sprite.sheets.length;
  const aspectRatio = `${String(sprite.columns * sprite.frameWidth)} / ${String(sprite.rows * sprite.frameHeight)}`;
  const openUrl = open === null ? null : sheets.sheetUrl(open);
  return (
    <>
      <ul aria-label={text.seekSprite} className="flex flex-wrap gap-2">
        {sprite.sheets.map((_, index) => {
          const url = sheets.sheetUrl(index);
          const name = text.seekSpriteSheet(index + 1, count);
          return (
            <li key={index} ref={observe} data-sheet={index}>
              <Button
                variant="ghost"
                aria-label={text.openSeekSpriteSheet(index + 1, count)}
                disabled={url === null}
                onClick={() => setOpen(index)}
                className="h-auto p-1"
              >
                <span
                  className="block w-replace-preview overflow-hidden rounded-sm bg-muted"
                  style={{ aspectRatio }}
                >
                  {url !== null && <img src={url} alt={name} className="size-full" />}
                </span>
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
              {open === null ? text.seekSprite : text.seekSpriteSheet(open + 1, count)}
            </DialogTitle>
          </DialogHeader>
          {openUrl !== null && open !== null && (
            <img
              src={openUrl}
              alt={text.seekSpriteSheet(open + 1, count)}
              className="w-full rounded-md"
              style={{ aspectRatio }}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
