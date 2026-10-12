import type { ThumbnailEditState } from "../../api/client";
import { t, type UiText } from "../../i18n";

// サムネイルの編集画面の規則（specs/045-comfyui-thumbnails/ui-design.md「Thumbnail editor」）。
// 画面の部品から切り出し、論理のテストで確かめる（docs/design-docs/web-testing.md
// 「Rules out of pages」）。

/** RunningState は実行が進んでいる間の状態である。 */
export type RunningState = Extract<
  ThumbnailEditState,
  "queued" | "preparing" | "running"
>;

/** isRunning は実行が進んでいる（待ち行列・準備中・生成中）かを返す。 */
export function isRunning(state: ThumbnailEditState): state is RunningState {
  return state === "queued" || state === "preparing" || state === "running";
}

/** runStateText は実行中の状態の文言である。 */
export function runStateText(state: RunningState): UiText {
  return t.player.thumbnailEdit.states[state];
}

/** LONG_WAIT_MS を過ぎて待ち行列か準備中なら、長い待ちの説明を出す。 */
export const LONG_WAIT_MS = 20_000;

/**
 * showsLongWait は長い待ちの説明を出すかを返す。待ち行列と準備中は実行を送った直後から
 * 続くので、送ってからの時間で数える。
 */
export function showsLongWait(state: ThumbnailEditState, elapsedMs: number): boolean {
  return (state === "queued" || state === "preparing") && elapsedMs >= LONG_WAIT_MS;
}

/**
 * elapsedNow は画面で進める経過時間（ミリ秒）である。サーバーの `elapsedMs` を受け取った
 * 時刻からの時間を足す。読むたびにサーバーの値へ合わせ直すので、読みの遅れより大きく
 * 戻ることはない。
 */
export function elapsedNow(serverMs: number, receivedAt: number, now: number): number {
  return Math.max(0, serverMs + Math.max(0, now - receivedAt));
}

/** BRUSH_STEPS はブラシの直径で、コマの短い辺に対する割合（2%・5%・10%）である。 */
export const BRUSH_STEPS = [0.02, 0.05, 0.1] as const;

/**
 * brushDiameter はブラシの段 `step`（0〜2）の直径を、コマの画素で返す。短い辺に対する
 * 割合なので、窓の大きさが変わっても 1 段がコマの同じ部分を覆う。
 */
export function brushDiameter(step: number, width: number, height: number): number {
  const index = Math.min(BRUSH_STEPS.length - 1, Math.max(0, Math.round(step)));
  return Math.max(1, (BRUSH_STEPS[index] ?? 0.05) * Math.min(width, height));
}

/** REFERENCE_MAX_BYTES は参考画像の上限（20 MiB）である。 */
export const REFERENCE_MAX_BYTES = 20 * 1024 * 1024;

const referenceTypes = new Set(["image/png", "image/jpeg", "image/webp"]);

/** referenceAccepted は参考画像として送れるファイル（PNG・JPEG・WebP、20 MB まで）かを返す。 */
export function referenceAccepted(file: Pick<File, "type" | "size">): boolean {
  return referenceTypes.has(file.type) && file.size <= REFERENCE_MAX_BYTES;
}

/**
 * shapeDiffers は生成した画像の形が動画（コマ）の形と違うかを返す。違えば、比べる箱は
 * サムネイルと同じく切り抜いて出し、その旨を書き添える。1% までの差は丸めの差として扱う。
 */
export function shapeDiffers(
  frameWidth: number,
  frameHeight: number,
  resultWidth: number,
  resultHeight: number,
): boolean {
  if (frameWidth <= 0 || frameHeight <= 0 || resultWidth <= 0 || resultHeight <= 0)
    return false;
  const frame = frameWidth / frameHeight;
  const result = resultWidth / resultHeight;
  return Math.abs(result - frame) / frame > 0.01;
}

/**
 * fitBox は幅 `width`・高さ `height` の場所に、比率 `aspect`（幅 / 高さ）の箱を全体が見える
 * いちばん大きな大きさで収める。
 */
export function fitBox(
  width: number,
  height: number,
  aspect: number,
): { width: number; height: number } {
  if (width <= 0 || height <= 0 || !(aspect > 0)) return { width: 0, height: 0 };
  if (width / height > aspect) return { width: height * aspect, height };
  return { width, height: width / aspect };
}
