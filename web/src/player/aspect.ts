import type { Video } from "../api/client";

/** 解像度が分からないときに枠が使う比率。 */
export const defaultAspectRatio = 16 / 9;

/**
 * 枠の比率の範囲。極端に細長い動画で枠が潰れたり、操作バーや状態表示が収まらなく
 * なったりしないよう、この範囲に丸める。範囲の外の動画は枠の中で上下か左右に余白が出る。
 */
const minAspectRatio = 9 / 16;
const maxAspectRatio = 21 / 9;

/**
 * frameAspectRatio は、プレイヤーの枠やシークのプレビューに使う横÷縦の比率を返す。
 * 縦長の動画は 1 未満になる。寸法が分からなければ 16:9 を返す。
 */
export function frameAspectRatio(
  width: number | undefined,
  height: number | undefined,
): number {
  if (width === undefined || height === undefined) return defaultAspectRatio;
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return defaultAspectRatio;
  }
  return Math.min(maxAspectRatio, Math.max(minAspectRatio, width / height));
}

/**
 * playerAspectRatio は、プレイヤーの枠に使う比率を選ぶ。再生を始めて分かった映像の比率が
 * いちばん確かで、次に解析が記録した表示の比率（画素の縦横比を反映済み）、最後に解像度の比を使う。
 */
export function playerAspectRatio(
  video: Pick<Video, "width" | "height" | "displayAspectRatio"> | undefined,
  mediaAspect: number | null,
): number {
  if (mediaAspect !== null) return frameAspectRatio(mediaAspect, 1);
  if (video?.displayAspectRatio !== undefined)
    return frameAspectRatio(video.displayAspectRatio, 1);
  return frameAspectRatio(video?.width, video?.height);
}

/**
 * playerNativeWidth は、広い画面でプレイヤーの幅の上限に使う動画の元の表示幅（px）を返す。
 * 解析が記録した表示の比率（画素の縦横比を反映済み）があれば高さに掛け、無ければ解像度の幅を
 * 使う。分からなければ undefined（上限は全体の上限だけになる）。
 */
export function playerNativeWidth(
  video: Pick<Video, "width" | "height" | "displayAspectRatio"> | undefined,
): number | undefined {
  const width = video?.width;
  const height = video?.height;
  if (height !== undefined && video?.displayAspectRatio !== undefined) {
    const shown = height * video.displayAspectRatio;
    if (Number.isFinite(shown) && shown > 0) return Math.round(shown);
  }
  if (width === undefined || !Number.isFinite(width) || width <= 0) return undefined;
  return width;
}
