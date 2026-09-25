/** 解像度が分からないときに枠が使う比率。 */
export const defaultAspectRatio = 16 / 9;

/**
 * 枠の比率の範囲。極端に細長い動画で枠が潰れたり、操作バーや状態表示が収まらなく
 * なったりしないよう、この範囲に丸める。範囲の外の動画は枠の中で上下か左右に余白が出る。
 */
const minAspectRatio = 9 / 21;
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
