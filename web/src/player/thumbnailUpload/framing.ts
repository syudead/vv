/** Share framing calculations between preview and upload; composeImage.ts owns rendering. */

/** Match the server's retained original-image long-side limit. */
export const COMPOSITION_LONG_SIDE = 2048;

export const MIN_ZOOM = 0.25;

export const MAX_ZOOM = 16;

/** Dragging stops while this share of the frame's side, or the whole image when it is smaller, still overlaps the frame. */
const KEEP_VISIBLE = 0.1;

export type Backdrop = "blur" | "black";

export type QuarterTurn = 0 | 90 | 180 | 270;

/** The image's centre offset from the frame's centre as fractions of the frame's width and height, its size as a multiple of the size that shows it whole, its clockwise turn, and what fills the frame where the image does not. */
export interface Fit {
  x: number;
  y: number;
  zoom: number;
  rotation: QuarterTurn;
  backdrop: Backdrop;
}

export const WHOLE_IMAGE: Fit = { x: 0, y: 0, zoom: 1, rotation: 0, backdrop: "blur" };

export interface Size {
  width: number;
  height: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** Prefer display aspect, then pixel dimensions, matching server framing; return null if unknown. */
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

/** The image's size after its quarter turn. */
export function turned(image: Size, rotation: QuarterTurn): Size {
  return rotation === 90 || rotation === 270
    ? { width: image.height, height: image.width }
    : { width: image.width, height: image.height };
}

/** Without a video aspect, the frame takes the turned image's own shape. */
export function compositionSize(image: Size, aspect: number | null): Size {
  const ratio = aspect ?? image.width / image.height;
  const long = COMPOSITION_LONG_SIDE;
  if (ratio >= 1) return { width: long, height: Math.max(1, Math.round(long / ratio)) };
  return { width: Math.max(1, Math.round(long * ratio)), height: long };
}

/** The output canvas size: the video's aspect, or the turned image's own when the video's is unknown. */
export function frameSize(
  image: Size,
  aspect: number | null,
  rotation: QuarterTurn,
): Size {
  return compositionSize(turned(image, rotation), aspect);
}

function wholeScale(image: Size, canvas: Size): number {
  return Math.min(canvas.width / image.width, canvas.height / image.height);
}

/** The zoom at which the centred, turned image just covers the frame. */
export function fillZoom(image: Size, canvas: Size, rotation: QuarterTurn): number {
  const shown = turned(image, rotation);
  const cover = Math.max(canvas.width / shown.width, canvas.height / shown.height);
  return cover / wholeScale(shown, canvas);
}

/** The turned image's rectangle on the canvas. A side within 1px of the frame's snaps to it so no seam shows. */
export function placeImage(image: Size, canvas: Size, fit: Fit): Rect {
  const shown = turned(image, fit.rotation);
  const scale = wholeScale(shown, canvas) * Math.max(MIN_ZOOM, fit.zoom);
  const width = snap(shown.width * scale, canvas.width);
  const height = snap(shown.height * scale, canvas.height);
  return {
    x: (canvas.width - width) / 2 + fit.x * canvas.width,
    y: (canvas.height - height) / 2 + fit.y * canvas.height,
    width,
    height,
  };
}

function snap(value: number, frame: number): number {
  return Math.abs(frame - value) <= 1 ? frame : value;
}

/** Whether the image leaves no part of the frame uncovered. */
export function coversFrame(image: Size, canvas: Size, fit: Fit): boolean {
  const placed = placeImage(image, canvas, fit);
  const slack = 0.5;
  return (
    placed.x <= slack &&
    placed.y <= slack &&
    placed.x + placed.width >= canvas.width - slack &&
    placed.y + placed.height >= canvas.height - slack
  );
}

/** Move by output-image pixels; the image stops while part of it still overlaps the frame. */
export function panBy(image: Size, canvas: Size, fit: Fit, dx: number, dy: number): Fit {
  const placed = placeImage(image, canvas, fit);
  const limit = (length: number, frame: number) =>
    0.5 + (length / 2 - Math.min(length, frame * KEEP_VISIBLE)) / frame;
  const limitX = limit(placed.width, canvas.width);
  const limitY = limit(placed.height, canvas.height);
  return {
    ...fit,
    x: clamp(fit.x + dx / canvas.width, -limitX, limitX),
    y: clamp(fit.y + dy / canvas.height, -limitY, limitY),
  };
}

/** Pull an axis to the frame's centre or align an edge with the frame's edge when it is within threshold output pixels. */
export function snapToFrame(image: Size, canvas: Size, fit: Fit, threshold: number): Fit {
  const placed = placeImage(image, canvas, fit);
  const axis = (offset: number, length: number, frame: number): number => {
    const pixels = offset * frame;
    const edge = (length - frame) / 2;
    const target = [0, edge, -edge].find(
      (candidate) => Math.abs(pixels - candidate) <= threshold,
    );
    return target === undefined ? offset : target / frame;
  };
  return {
    ...fit,
    x: axis(fit.x, placed.width, canvas.width),
    y: axis(fit.y, placed.height, canvas.height),
  };
}

/** Zoom about the frame's centre, so the part of the image there stays there. */
export function zoomTo(fit: Fit, zoom: number, max = MAX_ZOOM): Fit {
  const next = clamp(zoom, MIN_ZOOM, max);
  const ratio = next / Math.max(MIN_ZOOM, fit.zoom);
  return { ...fit, zoom: next, x: fit.x * ratio, y: fit.y * ratio };
}

export function wholeImage(fit: Fit): Fit {
  return { ...fit, x: 0, y: 0, zoom: 1 };
}

export function fillFrame(image: Size, canvas: Size, fit: Fit): Fit {
  return {
    ...fit,
    x: 0,
    y: 0,
    zoom: Math.max(MIN_ZOOM, fillZoom(image, canvas, fit.rotation)),
  };
}

export function rotateBy(fit: Fit, turn: 90 | -90): Fit {
  return { ...fit, rotation: ((fit.rotation + turn + 360) % 360) as QuarterTurn };
}

/** A very narrow image needs more than MAX_ZOOM to fill the frame; allow four times its fill. */
export function maxZoom(image: Size, canvas: Size, rotation: QuarterTurn): number {
  return Math.max(MAX_ZOOM, fillZoom(image, canvas, rotation) * 4);
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
