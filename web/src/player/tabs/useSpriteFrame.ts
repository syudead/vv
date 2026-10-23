import { useEffect, useRef, useState } from "react";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../../api/client";
import { seekSpriteCell } from "../../lib/seekSprite";
import type { ScrubFrameData } from "../../ui/ScrubPreview";

type SheetLoader = (url: string, sheet: number) => void;

/**
 * The seek sprite's cell at `positionMs`, as the seek preview draws it; null while the sprite or
 * its sheet is unavailable. Each sheet is read once and kept as an object URL until the sprite
 * URL changes or the caller unmounts.
 */
export function useSpriteFrame(
  spriteUrl: string | undefined,
  positionMs: number | null,
): ScrubFrameData | null {
  const [sprite, setSprite] = useState<SeekThumbnailSprite | null>(null);
  const [sheets, setSheets] = useState<ReadonlyMap<number, string>>(() => new Map());
  const loadSheet = useRef<SheetLoader | null>(null);

  useEffect(() => {
    setSprite(null);
    setSheets(new Map());
    if (spriteUrl === undefined) return;
    const controller = new AbortController();
    const requested = new Set<number>();
    const objectUrls: string[] = [];
    fetchSeekThumbnailSprite(spriteUrl, controller.signal).then(
      (loaded) => {
        if (!controller.signal.aborted) setSprite(loaded);
      },
      () => undefined,
    );
    loadSheet.current = (url, sheet) => {
      if (requested.has(sheet)) return;
      requested.add(sheet);
      fetchSeekThumbnailSheet(url, controller.signal).then(
        (blob) => {
          if (controller.signal.aborted) return;
          const objectUrl = URL.createObjectURL(blob);
          objectUrls.push(objectUrl);
          setSheets((previous) => new Map(previous).set(sheet, objectUrl));
        },
        () => undefined,
      );
    };
    return () => {
      controller.abort();
      loadSheet.current = null;
      for (const url of objectUrls) URL.revokeObjectURL(url);
    };
  }, [spriteUrl]);

  const cell =
    sprite === null || positionMs === null ? null : seekSpriteCell(sprite, positionMs);
  const sheetIndex = cell?.sheet;
  useEffect(() => {
    if (sprite === null || sheetIndex === undefined) return;
    const url = sprite.sheets[sheetIndex];
    if (url !== undefined) loadSheet.current?.(url, sheetIndex);
  }, [sprite, sheetIndex]);

  if (sprite === null || cell === null) return null;
  const url = sheets.get(cell.sheet);
  return url === undefined ? null : { url, sprite, cell };
}
