import { HEADER_READ_BYTES, readImageHeader } from "./imageHeader";
import { compositionSize, type Fit, placeImage, type Size } from "./framing";
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

/** Draw an opaque, theme-independent image matching the final JPEG. Use a blurred backdrop when supported, otherwise black. */
export function drawComposition(
  canvas: HTMLCanvasElement,
  image: UploadImage,
  aspect: number | null,
  fit: Fit,
): void {
  const size = compositionSize(image, aspect);
  if (canvas.width !== size.width) canvas.width = size.width;
  if (canvas.height !== size.height) canvas.height = size.height;
  const context = canvas.getContext("2d");
  if (context === null) return;
  context.imageSmoothingQuality = "high";
  const placed = placeImage(image, size, fit);
  // Clear previous content and composite transparency onto black, matching the saved JPEG.
  context.fillStyle = "black";
  context.fillRect(0, 0, size.width, size.height);
  if (placed.width < size.width || placed.height < size.height)
    drawBackdrop(context, image, size);
  context.drawImage(image.bitmap, placed.x, placed.y, placed.width, placed.height);
}

function drawBackdrop(
  context: CanvasRenderingContext2D,
  image: UploadImage,
  size: Size,
): void {
  if (!("filter" in context)) return;
  const cover = placeImage(image, size, { mode: "cover", zoom: 1.25, panX: 0, panY: 0 });
  context.save();
  context.filter = `blur(${String(Math.round(Math.max(size.width, size.height) * 0.03))}px)`;
  context.globalAlpha = 0.5;
  context.drawImage(image.bitmap, cover.x, cover.y, cover.width, cover.height);
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
