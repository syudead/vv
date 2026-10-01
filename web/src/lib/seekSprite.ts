import type { SeekThumbnailSprite } from "../api/client";

// シーク用スプライトのコマ選びと切り出しである。プレイヤーのシークバーの吹き出し
// （player/seekPreview.ts）と一覧のカードの帯（ui/ScrubPreview.tsx）が同じ規則で
// コマを選ぶように、ここを共有する（specs/032-card-scrub-preview/research.md R-1）。

/** SpriteCell は位置を受け持つコマと、それが載るシートの中の升目である。 */
export interface SpriteCell {
  frame: number;
  sheet: number;
  column: number;
  row: number;
}

/** SeekPosition は横位置から決めた、元動画の論理時刻である。 */
export interface SeekPosition {
  /** 矩形の左端からの距離（px）。矩形の外は端に丸める。 */
  localX: number;
  /** 矩形の幅に対する localX の割合（0〜1）。 */
  ratio: number;
  /** 論理時刻（ミリ秒）。右端は ceil(durationMs) - 1 に丸める。 */
  positionMs: number;
}

/**
 * seekPosition は矩形の中の横位置 clientX を、左端を 0ms、右端を末尾とする論理時刻に
 * 線形に対応させる。右端は ceil(durationMs) - 1 に丸め、末尾のコマの次（空き升目）を
 * 指さない。
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

/**
 * seekSpriteCell は元動画の論理時刻 positionMs（ミリ秒）のコマを、配置情報の間隔で
 * 決める（specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md §2）。間隔を
 * 固定値としては持たない。
 */
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

/** SpriteCellBackground は 1 コマの箱にシートを敷くときの背景の大きさと位置である。 */
export interface SpriteCellBackground {
  backgroundSize: string;
  backgroundPosition: string;
}

/**
 * spriteCellBackground は、1 コマの箱にシートを横 columns 倍・縦 rows 倍で敷いて、
 * 列と行の分だけずらす背景を返す。背景の位置の割合は（箱 − シート）の大きさに対する
 * 割合なので、column / (columns − 1) で箱の整数倍のずれになる。
 */
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
