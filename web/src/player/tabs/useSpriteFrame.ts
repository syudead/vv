import { useCallback, useEffect, useRef, useState } from "react";

import {
  fetchSeekThumbnailSheet,
  fetchSeekThumbnailSprite,
  type SeekThumbnailSprite,
} from "../../api/client";
import { seekSpriteCell, type SpriteCell } from "../../lib/seekSprite";
import type { ScrubFrameData } from "../../ui/ScrubPreview";

/** The requests and object URLs of one sprite URL, released together when it changes. */
interface SpriteSession {
  url: string;
  signal: AbortSignal;
  sprite: SeekThumbnailSprite | null;
  spriteRequested: boolean;
  sheetsRequested: Set<number>;
  objectUrls: string[];
}

interface LoadedSprite {
  source: string;
  sprite: SeekThumbnailSprite;
  sheets: ReadonlyMap<number, string>;
  failedSheets: ReadonlySet<number>;
}

export interface SpriteSheets {
  /** The layout of the current sprite URL once read. */
  sprite: SeekThumbnailSprite | null;
  /** The layout could not be read; `retry` reads it again. */
  failed: boolean;
  /** Some sheet could not be read; `retry` reads the failed sheets again. */
  sheetFailed: boolean;
  /** The cell drawn from its sheet, or null while that sheet is not loaded. */
  frame: (cell: SpriteCell) => ScrubFrameData | null;
  /** Read a sheet once; a failed sheet may be requested again. */
  requestSheet: (index: number) => void;
  retry: () => void;
}

function withSheet(
  previous: LoadedSprite | null,
  source: string,
  index: number,
  objectUrl: string | null,
): LoadedSprite | null {
  if (previous === null || previous.source !== source) return previous;
  const sheets = new Map(previous.sheets);
  const failedSheets = new Set(previous.failedSheets);
  if (objectUrl === null) failedSheets.add(index);
  else {
    sheets.set(index, objectUrl);
    failedSheets.delete(index);
  }
  return { ...previous, sheets, failedSheets };
}

/**
 * The seek sprite of `spriteUrl` and its sheets as object URLs. The layout is read once `wanted`
 * holds, each sheet only when requested, and everything is released when the URL changes or the
 * caller unmounts.
 */
export function useSpriteSheets(
  spriteUrl: string | undefined,
  wanted: boolean,
): SpriteSheets {
  const [loaded, setLoaded] = useState<LoadedSprite | null>(null);
  const [failedSource, setFailedSource] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const session = useRef<SpriteSession | null>(null);

  useEffect(() => {
    if (spriteUrl === undefined) return;
    const controller = new AbortController();
    const current: SpriteSession = {
      url: spriteUrl,
      signal: controller.signal,
      sprite: null,
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
      setFailedSource(null);
    };
  }, [spriteUrl]);

  useEffect(() => {
    const current = session.current;
    if (!wanted || current === null || current.spriteRequested) return;
    current.spriteRequested = true;
    fetchSeekThumbnailSprite(current.url, current.signal).then(
      (sprite) => {
        if (current.signal.aborted) return;
        current.sprite = sprite;
        setLoaded({
          source: current.url,
          sprite,
          sheets: new Map(),
          failedSheets: new Set(),
        });
      },
      () => {
        current.spriteRequested = false;
        if (!current.signal.aborted) setFailedSource(current.url);
      },
    );
  }, [spriteUrl, wanted, attempt]);

  const requestSheet = useCallback((index: number) => {
    const current = session.current;
    const url = current?.sprite?.sheets[index];
    if (current === null || url === undefined || current.sheetsRequested.has(index))
      return;
    current.sheetsRequested.add(index);
    fetchSeekThumbnailSheet(url, current.signal).then(
      (blob) => {
        if (current.signal.aborted) return;
        const objectUrl = URL.createObjectURL(blob);
        current.objectUrls.push(objectUrl);
        setLoaded((previous) => withSheet(previous, current.url, index, objectUrl));
      },
      () => {
        current.sheetsRequested.delete(index);
        if (!current.signal.aborted) {
          setLoaded((previous) => withSheet(previous, current.url, index, null));
        }
      },
    );
  }, []);

  // The first render after a URL change still holds the previous video's sprite until cleanup runs.
  const own = loaded !== null && loaded.source === spriteUrl ? loaded : null;
  const failed = failedSource !== null && failedSource === spriteUrl;
  return {
    sprite: own?.sprite ?? null,
    failed,
    sheetFailed: own !== null && own.failedSheets.size > 0,
    frame: (cell) => {
      const url = own?.sheets.get(cell.sheet);
      return own === null || url === undefined ? null : { url, sprite: own.sprite, cell };
    },
    requestSheet,
    retry: () => {
      if (failed) {
        setFailedSource(null);
        setAttempt((n) => n + 1);
      }
      for (const index of own?.failedSheets ?? []) requestSheet(index);
    },
  };
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
  const sheets = useSpriteSheets(spriteUrl, positionMs !== null);
  const { sprite, requestSheet } = sheets;
  const cell =
    sprite === null || positionMs === null ? null : seekSpriteCell(sprite, positionMs);
  const sheetIndex = cell?.sheet;
  const frame = cell?.frame;
  useEffect(() => {
    if (sheetIndex !== undefined) requestSheet(sheetIndex);
  }, [requestSheet, sprite, sheetIndex, frame]);
  return cell === null ? null : sheets.frame(cell);
}
