import type { Rect } from "./faceFraming";

const FACE_JPEG_QUALITY = 0.92;

/**
 * The long side every cut is resampled to, matching the server's PersonImageMaxSide. A Retina profile shows the face
 * about 480 × 720 device pixels, so a smaller cut is enlarged here, smoothly, rather than its JPEG blocks on screen.
 */
export const FACE_IMAGE_SIDE = 800;

/** Cut rect out of the frame and resample it to FACE_IMAGE_SIDE on its long side, as the JPEG sent as the face. */
export function encodeFaceFrame(image: HTMLImageElement, rect: Rect): Promise<Blob> {
  const scale = FACE_IMAGE_SIDE / Math.max(rect.width, rect.height, 1);
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(rect.width * scale));
  canvas.height = Math.max(1, Math.round(rect.height * scale));
  const context = canvas.getContext("2d");
  if (context === null) return Promise.reject(new Error("Canvas is unavailable."));
  context.imageSmoothingEnabled = true;
  context.imageSmoothingQuality = "high";
  context.drawImage(
    image,
    rect.x,
    rect.y,
    rect.width,
    rect.height,
    0,
    0,
    canvas.width,
    canvas.height,
  );
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (blob === null) reject(new Error("Could not encode the face."));
        else resolve(blob);
      },
      "image/jpeg",
      FACE_JPEG_QUALITY,
    );
  });
}
