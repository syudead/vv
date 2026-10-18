import type { SeekThumbnailSprite } from "../api/client";

// Share frame selection between the player seek preview and card scrub preview.

export interface SpriteCell {
  frame: number;
  sheet: number;
  column: number;
  row: number;
}

export interface SeekPosition {
  localX: number;
  ratio: number;
  /** Clamp the right edge to ceil(durationMs) - 1. */
  positionMs: number;
}

/**
 * Map horizontal position linearly to logical video time; clamp the end to avoid selecting an
 * unused sprite cell.
 */
export function seekPosition(
  clientX: number,
  rect: Pick<DOMRect, "left" | "width">,
  durationMs: number,
): SeekPosition {
  const localX = Math.min(Math.max(clientX - rect.left, 0), rect.width);
  const ratio = rect.width > 0 ? localX / rect.width : 0;
  const lastPositionMs = Math.max(0, Math.ceil(durationMs) - 1);
  const positionMs = Math.min(lastPositionMs, Math.round(ratio * durationMs));
  return { localX, ratio, positionMs };
}

/** Use the interval from sprite metadata, never a fixed interval. */
export function seekSpriteCell(
  sprite: Pick<SeekThumbnailSprite, "intervalMs" | "frameCount" | "columns" | "rows">,
  positionMs: number,
): SpriteCell {
  const frame = Math.min(
    Math.max(0, Math.floor(positionMs / sprite.intervalMs)),
    sprite.frameCount - 1,
  );
  const perSheet = sprite.columns * sprite.rows;
  const index = frame % perSheet;
  return {
    frame,
    sheet: Math.floor(frame / perSheet),
    column: index % sprite.columns,
    row: Math.floor(index / sprite.columns),
  };
}

export interface SpriteCellBackground {
  backgroundSize: string;
  backgroundPosition: string;
}

/** Background percentages use box minus sheet size, so column / (columns - 1) shifts by whole cells. */
export function spriteCellBackground(
  cell: Pick<SpriteCell, "column" | "row">,
  layout: Pick<SeekThumbnailSprite, "columns" | "rows">,
): SpriteCellBackground {
  return {
    backgroundSize: `${String(layout.columns * 100)}% ${String(layout.rows * 100)}%`,
    backgroundPosition: `${String(offsetPercent(cell.column, layout.columns))}% ${String(offsetPercent(cell.row, layout.rows))}%`,
  };
}

function offsetPercent(index: number, count: number): number {
  return count > 1 ? (index / (count - 1)) * 100 : 0;
}
