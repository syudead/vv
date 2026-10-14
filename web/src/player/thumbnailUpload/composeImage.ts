import { HEADER_READ_BYTES, readImageHeader } from "./imageHeader";
import { compositionSize, type Fit, placeImage, type Size } from "./framing";
import { UPLOAD_MAX_PIXELS } from "./uploadRules";

/**
 * 選んだ画像を読み、サムネイルの枠に収めた絵を描く。プレビューの canvas と送る画像は同じ
 * drawComposition で描く（docs/design-docs/thumbnail-editing.md「Uploaded images」）。
 * jsdom には canvas も画像の復号も無いので、ページの試験はこのモジュールを差し替える。
 */

/** UploadImage は復号した元の画像で、大きさは向き（EXIF）を反映した表示上の値である。 */
export interface UploadImage extends Size {
  bitmap: ImageBitmap;
}

/** LoadRefusal はプレビューを出す前に画像を断る理由で、サーバーの理由の名前と同じである。 */
export type LoadRefusal = "thumbnail_upload_invalid" | "thumbnail_upload_too_large";

export type LoadResult = { image: UploadImage } | { refusal: LoadRefusal };

/**
 * loadUploadImage はファイルを復号する。見出しがアニメーションの WebP を名乗るか、復号できなければ
 * `thumbnail_upload_invalid`、見出しか復号した画像が 1 億 2000 万画素を越えれば
 * `thumbnail_upload_too_large` で断る。見出しで断れるものは復号の前に断る。
 */
export async function loadUploadImage(file: Blob): Promise<LoadResult> {
  const head = new Uint8Array(await file.slice(0, HEADER_READ_BYTES).arrayBuffer());
  const header = readImageHeader(head);
  if (header?.animated === true) return { refusal: "thumbnail_upload_invalid" };
  if (header !== null && header.width * header.height > UPLOAD_MAX_PIXELS)
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

/**
 * drawComposition は `canvas` を送る画像の大きさにし、比率 `aspect`（`null` は画像自身）の枠へ
 * 画像を形 `fit` で描く。余白は黒の上に、枠を覆うまで広げてぼかした画像を半分の濃さで重ねる。
 * サムネイルは透けない JPEG になり、明るいテーマにも暗いテーマにも同じファイルを出すので、
 * テーマの色は使わない。ぼかしを描けないブラウザーでは黒だけになり、プレビューも同じになる。
 */
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
  if (placed.width < size.width || placed.height < size.height) {
    context.fillStyle = "black";
    context.fillRect(0, 0, size.width, size.height);
    drawBackdrop(context, image, size);
  }
  context.drawImage(image.bitmap, placed.x, placed.y, placed.width, placed.height);
}

/** drawBackdrop は余白に、枠を覆うまで 1.25 倍に広げてぼかした画像を半分の濃さで描く。 */
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

/** encodeComposition は drawComposition と同じ絵を PNG にする。 */
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
