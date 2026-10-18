/** Share framing calculations between preview and upload; composeImage.ts owns rendering. */

/** Match the server's retained original-image long-side limit. */
export const COMPOSITION_LONG_SIDE = 2048;

export const MAX_ZOOM = 4;

/** Cover pan uses -1..1 fractions of available movement; endpoints expose the image's left/top or right/bottom edge. */
export type Fit =
  { mode: "contain" } | { mode: "cover"; zoom: number; panX: number; panY: number };

export const CENTERED_COVER: Fit = { mode: "cover", zoom: 1, panX: 0, panY: 0 };

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

export function compositionSize(image: Size, aspect: number | null): Size {
  const ratio = aspect ?? image.width / image.height;
  const long = COMPOSITION_LONG_SIDE;
  if (ratio >= 1) return { width: long, height: Math.max(1, Math.round(long / ratio)) };
  return { width: Math.max(1, Math.round(long * ratio)), height: long };
}

/** Snap contain edges within 1px to avoid seams; clamp cover pan so the image always fills the frame. */
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

function snap(value: number, frame: number): number {
  return frame - value <= 1 ? frame : value;
}

/** Pan in output-image pixels; axes with no overflow remain fixed. */
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
