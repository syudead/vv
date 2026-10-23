import { useEffect, useRef, useState } from "react";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../../api/client";
import { seekSpriteCell } from "../../lib/seekSprite";
import type { ScrubFrameData } from "../../ui/ScrubPreview";

/** The requests and object URLs of one sprite URL, released together when it changes. */
interface SpriteSession {
  url: string;
  signal: AbortSignal;
  spriteRequested: boolean;
  sheetsRequested: Set<number>;
  objectUrls: string[];
}

interface LoadedSprite {
  source: string;
  sprite: SeekThumbnailSprite;
  sheets: ReadonlyMap<number, string>;
}

/**
 * The seek sprite's cell at `positionMs`, as the seek preview draws it; null while the position is
 * null or the sprite or its sheet is unavailable. The sprite is read only once a position exists,
 * and each sheet once, kept as an object URL until the sprite URL changes or the caller unmounts.
 * A failed sheet is requested again when the position reaches another frame.
 */
export function useSpriteFrame(
  spriteUrl: string | undefined,
  positionMs: number | null,
): ScrubFrameData | null {
  const [loaded, setLoaded] = useState<LoadedSprite | null>(null);
  const session = useRef<SpriteSession | null>(null);

  useEffect(() => {
    if (spriteUrl === undefined) return;
    const controller = new AbortController();
    const current: SpriteSession = {
      url: spriteUrl,
      signal: controller.signal,
      spriteRequested: false,
      sheetsRequested: new Set(),
      objectUrls: [],
    };
    session.current = current;
    return () => {
      controller.abort();
      session.current = null;
      for (const url of current.objectUrls) URL.revokeObjectURL(url);
      setLoaded(null);
    };
  }, [spriteUrl]);

  const wantsFrame = positionMs !== null;
  useEffect(() => {
    const current = session.current;
    if (!wantsFrame || current === null || current.spriteRequested) return;
    current.spriteRequested = true;
    fetchSeekThumbnailSprite(current.url, current.signal).then(
      (sprite) => {
        if (!current.signal.aborted)
          setLoaded({ source: current.url, sprite, sheets: new Map() });
      },
      () => {
        current.spriteRequested = false;
      },
    );
  }, [spriteUrl, wantsFrame]);

  // The first render after a URL change still holds the previous video's sprite until cleanup runs.
  const sprite = loaded !== null && loaded.source === spriteUrl ? loaded.sprite : null;
  const cell =
    sprite === null || positionMs === null ? null : seekSpriteCell(sprite, positionMs);
  const sheetIndex = cell?.sheet;
  const frame = cell?.frame;
  useEffect(() => {
    const current = session.current;
    if (sprite === null || sheetIndex === undefined || frame === undefined) return;
    const url = sprite.sheets[sheetIndex];
    if (current === null || url === undefined || current.sheetsRequested.has(sheetIndex))
      return;
    current.sheetsRequested.add(sheetIndex);
    fetchSeekThumbnailSheet(url, current.signal).then(
      (blob) => {
        if (current.signal.aborted) return;
        const objectUrl = URL.createObjectURL(blob);
        current.objectUrls.push(objectUrl);
        setLoaded((previous) =>
          previous === null || previous.source !== current.url
            ? previous
            : {
                ...previous,
                sheets: new Map(previous.sheets).set(sheetIndex, objectUrl),
              },
        );
      },
      () => {
        current.sheetsRequested.delete(sheetIndex);
      },
    );
  }, [sprite, sheetIndex, frame]);

  if (sprite === null || cell === null || loaded === null) return null;
  const url = loaded.sheets.get(cell.sheet);
  return url === undefined ? null : { url, sprite, cell };
}
