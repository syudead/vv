import type { Rect } from "./faceFraming";

const FACE_JPEG_QUALITY = 0.92;

/** Cut rect out of the frame at its own pixels, as the JPEG sent as the person's face. */
export function encodeFaceFrame(image: HTMLImageElement, rect: Rect): Promise<Blob> {
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(rect.width));
  canvas.height = Math.max(1, Math.round(rect.height));
  const context = canvas.getContext("2d");
  if (context === null) return Promise.reject(new Error("Canvas is unavailable."));
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
