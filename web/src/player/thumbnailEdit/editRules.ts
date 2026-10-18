import type { ThumbnailEdit, ThumbnailEditState } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";

// 「Rules out of pages」）。

export type RunningState = Extract<
  ThumbnailEditState,
  "queued" | "preparing" | "running"
>;

export function isRunning(state: ThumbnailEditState): state is RunningState {
  return state === "queued" || state === "preparing" || state === "running";
}

export function runStateText(state: RunningState): UiText {
  return t.player.thumbnailEdit.states[state];
}

export interface SourceWords {
  detail: UiText;
  paintingHint: UiText;
  sourceLabel: UiText;
}

/** Derive labels from session source/position only; page thumbnail data may change before the image is frozen. */
export function sourceWords(
  edit: Pick<ThumbnailEdit, "source" | "positionMs">,
): SourceWords {
  const text = t.player.thumbnailEdit;
  const time = edit.positionMs === undefined ? null : formatDuration(edit.positionMs);
  if (edit.source === "thumbnail")
    return {
      detail: time === null ? text.thumbnailSavedImage : text.thumbnailFrameAt(time),
      paintingHint: text.paintingHintImage,
      sourceLabel: text.thumbnail,
    };
  return {
    detail: text.frameAt(time ?? formatDuration(0)),
    paintingHint: text.paintingHint,
    sourceLabel: text.frame,
  };
}

export const LONG_WAIT_MS = 20_000;

export function showsLongWait(state: ThumbnailEditState, elapsedMs: number): boolean {
  return (state === "queued" || state === "preparing") && elapsedMs >= LONG_WAIT_MS;
}

/** Advance from the server's elapsed value and receipt time; each poll resynchronizes the clock. */
export function elapsedNow(serverMs: number, receivedAt: number, now: number): number {
  return Math.max(0, serverMs + Math.max(0, now - receivedAt));
}

export const BRUSH_STEPS = [0.02, 0.05, 0.1] as const;

/** Scale brush diameter by the frame's short side so resizing does not change the painted area. */
export function brushDiameter(step: number, width: number, height: number): number {
  const index = Math.min(BRUSH_STEPS.length - 1, Math.max(0, Math.round(step)));
  return Math.max(1, (BRUSH_STEPS[index] ?? 0.05) * Math.min(width, height));
}

export const REFERENCE_MAX_BYTES = 20 * 1024 * 1024;

const referenceTypes = new Set(["image/png", "image/jpeg", "image/webp"]);

export function referenceAccepted(file: Pick<File, "type" | "size">): boolean {
  return referenceTypes.has(file.type) && file.size <= REFERENCE_MAX_BYTES;
}

/** Encode markup characters while preserving the browser-generated blob URL. */
export function encodeReferenceURL(url: string): string {
  return url
    .replace(/</g, "%3C")
    .replace(/>/g, "%3E")
    .replace(/"/g, "%22")
    .replace(/'/g, "%27");
}

/** Treat aspect-ratio differences up to 1% as rounding; larger differences need a crop notice. */
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

export function fitBox(
  width: number,
  height: number,
  aspect: number,
): { width: number; height: number } {
  if (width <= 0 || height <= 0 || !(aspect > 0)) return { width: 0, height: 0 };
  if (width / height > aspect) return { width: height * aspect, height };
  return { width, height: width / aspect };
}
