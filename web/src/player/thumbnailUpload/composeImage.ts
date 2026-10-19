import { HEADER_READ_BYTES, readImageHeader } from "./imageHeader";
import {
  coversFrame,
  type Fit,
  fillZoom,
  frameSize,
  placeImage,
  type Rect,
  type Size,
  turned,
} from "./framing";
import { UPLOAD_MAX_PIXELS } from "./uploadRules";

/** Share drawComposition between preview and output; page tests replace this boundary because jsdom lacks canvas and decoding. */

/** Decoded dimensions include EXIF orientation. */
export interface UploadImage extends Size {
  bitmap: ImageBitmap;
}

export type LoadRefusal = "thumbnail_upload_invalid" | "thumbnail_upload_too_large";

export type LoadResult = { image: UploadImage } | { refusal: LoadRefusal };

/** Validate headers before decoding; reject unknown formats, animated WebP, and images exceeding 120 million pixels. */
export async function loadUploadImage(file: Blob): Promise<LoadResult> {
  const head = new Uint8Array(await file.slice(0, HEADER_READ_BYTES).arrayBuffer());
  const header = readImageHeader(head);
  // The server sees only the composed PNG, so validate the original format here.
  if (header === null || header.animated) return { refusal: "thumbnail_upload_invalid" };
  if (header.width * header.height > UPLOAD_MAX_PIXELS)
    return { refusal: "thumbnail_upload_too_large" };
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    return { refusal: "thumbnail_upload_invalid" };
  }
  if (bitmap.width * bitmap.height > UPLOAD_MAX_PIXELS) {
    bitmap.close();
    return { refusal: "thumbnail_upload_too_large" };
  }
  return { image: { bitmap, width: bitmap.width, height: bitmap.height } };
}

/** Draw an opaque, theme-independent image matching the final JPEG. The uncovered frame is black under a blurred copy when the fit asks for it and the context can blur. */
export function drawComposition(
  canvas: HTMLCanvasElement,
  image: UploadImage,
  aspect: number | null,
  fit: Fit,
): void {
  const size = frameSize(image, aspect, fit.rotation);
  if (canvas.width !== size.width) canvas.width = size.width;
  if (canvas.height !== size.height) canvas.height = size.height;
  const context = canvas.getContext("2d");
  if (context === null) return;
  context.imageSmoothingQuality = "high";
  // Clear previous content and composite transparency onto black, matching the saved JPEG.
  context.fillStyle = "black";
  context.fillRect(0, 0, size.width, size.height);
  if (fit.backdrop === "blur" && !coversFrame(image, size, fit))
    drawBackdrop(context, image, size, fit);
  drawPlaced(context, image, placeImage(image, size, fit), fit);
}

/** Draw the whole image, without the frame's fill, on a canvas three frames wide and high with the frame in the middle, so the preview can show what falls outside the frame. */
export function drawOverflow(
  canvas: HTMLCanvasElement,
  image: UploadImage,
  aspect: number | null,
  fit: Fit,
  scale: number,
): void {
  const size = frameSize(image, aspect, fit.rotation);
  const width = Math.max(1, Math.round(size.width * 3 * scale));
  const height = Math.max(1, Math.round(size.height * 3 * scale));
  if (canvas.width !== width) canvas.width = width;
  if (canvas.height !== height) canvas.height = height;
  const context = canvas.getContext("2d");
  if (context === null) return;
  context.clearRect(0, 0, width, height);
  context.save();
  context.scale(scale, scale);
  context.translate(size.width, size.height);
  drawPlaced(context, image, placeImage(image, size, fit), fit);
  context.restore();
}

/** placed is the turned image's rectangle; a turn draws the bitmap rotated about its centre. */
function drawPlaced(
  context: CanvasRenderingContext2D,
  image: UploadImage,
  placed: Rect,
  fit: Fit,
): void {
  if (fit.rotation === 0) {
    context.drawImage(image.bitmap, placed.x, placed.y, placed.width, placed.height);
    return;
  }
  const upright = turned(placed, fit.rotation);
  context.save();
  context.translate(placed.x + placed.width / 2, placed.y + placed.height / 2);
  context.rotate((fit.rotation * Math.PI) / 180);
  context.drawImage(
    image.bitmap,
    -upright.width / 2,
    -upright.height / 2,
    upright.width,
    upright.height,
  );
  context.restore();
}

/** A blurred copy over-filling the frame, turned like the image. */
function drawBackdrop(
  context: CanvasRenderingContext2D,
  image: UploadImage,
  size: Size,
  fit: Fit,
): void {
  if (!("filter" in context)) return;
  const cover = { ...fit, x: 0, y: 0, zoom: fillZoom(image, size, fit.rotation) * 1.25 };
  context.save();
  context.filter = `blur(${String(Math.round(Math.max(size.width, size.height) * 0.03))}px)`;
  context.globalAlpha = 0.5;
  drawPlaced(context, image, placeImage(image, size, cover), fit);
  context.restore();
}

export function encodeComposition(
  image: UploadImage,
  aspect: number | null,
  fit: Fit,
): Promise<Blob> {
  const canvas = document.createElement("canvas");
  drawComposition(canvas, image, aspect, fit);
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob === null) reject(new Error("the composition could not be encoded"));
      else resolve(blob);
    }, "image/png");
  });
}
