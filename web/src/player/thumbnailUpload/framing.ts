/**
 * 手元の画像をサムネイルの枠に収める形の計算である（docs/design-docs/thumbnail-editing.md
 * 「Uploaded images」）。プレビューと送る画像は同じ計算から同じ絵を描くので、見えていたとおりの
 * 画像がサムネイルになる。描画そのものは composeImage.ts が行う。
 */

/** COMPOSITION_LONG_SIDE は送る画像の長辺（px）で、サーバーが残す元の画像の上限と同じである。 */
export const COMPOSITION_LONG_SIDE = 2048;

/** MAX_ZOOM は「枠いっぱいに広げる」の拡大率の上限である。 */
export const MAX_ZOOM = 4;

/**
 * Fit は画像の収め方である。`contain` は全体を枠に収めて足りない部分を余白で埋め、`cover` は
 * 枠いっぱいに広げて `zoom`（1〜MAX_ZOOM）と `panX`・`panY`（-1〜1）で見せる範囲を決める。
 * pan は動かせる幅に対する割合で、-1 は画像の左端（上端）、1 は右端（下端）を見せる。
 */
export type Fit =
  { mode: "contain" } | { mode: "cover"; zoom: number; panX: number; panY: number };

/** CENTERED_COVER は「枠いっぱいに広げる」に切り替えたときの最初の形（等倍・中央）である。 */
export const CENTERED_COVER: Fit = { mode: "cover", zoom: 1, panX: 0, panY: 0 };

export interface Size {
  width: number;
  height: number;
}

/** Rect は送る画像の中で、元の画像を描く位置と大きさ（px）である。 */
export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/**
 * frameAspect は画像を収める枠の縦横比（横÷縦）を返す。サーバーが画像を合わせる比率と同じく、
 * 表示上の比率があればそれ、無ければ幅と高さから求め、どちらも分からなければ `null` である。
 */
export function frameAspect(video: {
  displayAspectRatio?: number;
  width?: number;
  height?: number;
}): number | null {
  if (video.displayAspectRatio !== undefined && video.displayAspectRatio > 0)
    return video.displayAspectRatio;
  if (
    video.width !== undefined &&
    video.height !== undefined &&
    video.width > 0 &&
    video.height > 0
  )
    return video.width / video.height;
  return null;
}

/**
 * compositionSize は送る画像の大きさを返す。比率 `aspect` の、長辺が COMPOSITION_LONG_SIDE の
 * 大きさで、`aspect` が `null`（動画の比率が分からない）なら画像自身の比率にする。
 */
export function compositionSize(image: Size, aspect: number | null): Size {
  const ratio = aspect ?? image.width / image.height;
  const long = COMPOSITION_LONG_SIDE;
  if (ratio >= 1) return { width: long, height: Math.max(1, Math.round(long / ratio)) };
  return { width: Math.max(1, Math.round(long * ratio)), height: long };
}

/**
 * placeImage は大きさ `canvas` の送る画像の中で、大きさ `image` の元の画像を形 `fit` で描く位置を
 * 返す。`contain` で枠との差が 1px 以内の辺は枠いっぱいに描き、比率の同じ画像に余白の線を
 * 出さない。`cover` の位置は、画像が常に枠を覆うよう -1〜1 に収めて計算する。
 */
export function placeImage(image: Size, canvas: Size, fit: Fit): Rect {
  if (fit.mode === "contain") {
    const scale = Math.min(canvas.width / image.width, canvas.height / image.height);
    const width = snap(image.width * scale, canvas.width);
    const height = snap(image.height * scale, canvas.height);
    return {
      x: (canvas.width - width) / 2,
      y: (canvas.height - height) / 2,
      width,
      height,
    };
  }
  const scale =
    Math.max(canvas.width / image.width, canvas.height / image.height) *
    clampZoom(fit.zoom);
  const width = image.width * scale;
  const height = image.height * scale;
  return {
    x: ((canvas.width - width) / 2) * (1 + clampPan(fit.panX)),
    y: ((canvas.height - height) / 2) * (1 + clampPan(fit.panY)),
    width,
    height,
  };
}

/** snap は `value` が `frame` との差 1px 以内なら `frame` を返す。 */
function snap(value: number, frame: number): number {
  return frame - value <= 1 ? frame : value;
}

/**
 * panBy は `cover` の形 `fit` を、送る画像の画素で (dx, dy) だけ画像を動かした形にする。
 * 動かせる幅が無い向きは動かない。
 */
export function panBy(image: Size, canvas: Size, fit: Fit, dx: number, dy: number): Fit {
  if (fit.mode !== "cover") return fit;
  const placed = placeImage(image, canvas, fit);
  const travelX = (placed.width - canvas.width) / 2;
  const travelY = (placed.height - canvas.height) / 2;
  return {
    ...fit,
    panX: travelX > 0 ? clampPan(fit.panX - dx / travelX) : 0,
    panY: travelY > 0 ? clampPan(fit.panY - dy / travelY) : 0,
  };
}

/** zoomTo は `cover` の形 `fit` の拡大率を `zoom`（1〜MAX_ZOOM に収める）にする。 */
export function zoomTo(fit: Fit, zoom: number): Fit {
  if (fit.mode !== "cover") return fit;
  return { ...fit, zoom: clampZoom(zoom) };
}

function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(1, zoom));
}

function clampPan(pan: number): number {
  return Math.min(1, Math.max(-1, pan));
}
