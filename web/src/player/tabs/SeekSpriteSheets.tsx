import { LayoutGrid } from "lucide-react";
import { useEffect, useState } from "react";

import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../../ui/shadcn/dialog";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { ReadFailure } from "./panelParts";
import { type SpriteSheets, useSpriteSheets } from "./useSpriteFrame";

/**
 * `View seek sprite`, which shows the sprite's sheets as they are in a dialog, for viewing only.
 * The sprite and its sheets are read when the dialog first opens.
 */
export default function SeekSpriteSheets({ spriteUrl }: { spriteUrl: string }) {
  const [open, setOpen] = useState(false);
  const [wanted, setWanted] = useState(false);
  const sheets = useSpriteSheets(spriteUrl, wanted);
  const text = t.player.tabs;
  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="-ml-2"
        onClick={() => {
          setWanted(true);
          setOpen(true);
        }}
      >
        <LayoutGrid aria-hidden="true" />
        {text.viewSeekSprite}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        {/* Wide, as FormDialog's `wide` size, so a sheet's frames stay readable. */}
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{text.seekSprite}</DialogTitle>
          </DialogHeader>
          <SheetImages sheets={sheets} />
        </DialogContent>
      </Dialog>
    </>
  );
}

function SheetImages({ sheets }: { sheets: SpriteSheets }) {
  const { sprite, requestSheet } = sheets;
  const text = t.player.tabs;
  useEffect(() => {
    sprite?.sheets.forEach((_, index) => requestSheet(index));
  }, [sprite, requestSheet]);

  if (sheets.failed)
    return <ReadFailure text={text.seekSpriteFailed} onRetry={sheets.retry} />;
  if (sprite === null)
    return (
      <Skeleton
        role="status"
        aria-label={text.seekSpriteLoading}
        className="aspect-video w-full"
      />
    );
  const count = sprite.sheets.length;
  const aspectRatio = `${String(sprite.columns * sprite.frameWidth)} / ${String(sprite.rows * sprite.frameHeight)}`;
  return (
    <div className="flex flex-col gap-3">
      {sprite.sheets.map((_, index) => {
        const url = sheets.sheetUrl(index);
        const name = text.seekSpriteSheet(index + 1, count);
        return url === null ? (
          <Skeleton key={index} className="w-full" style={{ aspectRatio }} />
        ) : (
          <img
            key={index}
            src={url}
            alt={name}
            className="w-full rounded-md"
            style={{ aspectRatio }}
          />
        );
      })}
      {sheets.sheetFailed && (
        <ReadFailure text={text.seekSpriteFailed} onRetry={sheets.retry} />
      )}
    </div>
  );
}
