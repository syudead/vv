// 塗りの層の筆跡と、送るマスクの組み立て（specs/045-comfyui-thumbnails/ui-design.md
// 「Compose」、contracts/workflow.md「Workflows」）。座標と太さはコマの画素で持つので、
// 窓の大きさが変わっても筆跡はコマの同じ場所に残る。

/** Stroke は 1 回の筆跡である。`erase` は塗りを消す。 */
export interface Stroke {
  mode: "paint" | "erase";
  /** ブラシの直径（コマの画素）。 */
  size: number;
  /** 通った点（コマの画素）。1 点だけなら丸い点になる。 */
  points: { x: number; y: number }[];
}

/** PaintContext は筆跡を描くのに使う CanvasRenderingContext2D の部分である。 */
export type PaintContext = Pick<
  CanvasRenderingContext2D,
  | "beginPath"
  | "moveTo"
  | "lineTo"
  | "arc"
  | "fill"
  | "stroke"
  | "globalCompositeOperation"
  | "strokeStyle"
  | "fillStyle"
  | "lineWidth"
  | "lineCap"
  | "lineJoin"
>;

/**
 * drawStroke は筆跡を `context` に描く。塗りは `color` で重ね、消しは
 * `eraseColor` で消す（null なら表示の層として destination-out で抜き、マスクは黒で上塗りする）。
 */
export function drawStroke(
  context: PaintContext,
  stroke: Stroke,
  color: string,
  eraseColor: string | null,
): void {
  const [first, ...rest] = stroke.points;
  if (first === undefined) return;
  const erasing = stroke.mode === "erase";
  context.globalCompositeOperation =
    erasing && eraseColor === null ? "destination-out" : "source-over";
  const ink = erasing && eraseColor !== null ? eraseColor : color;
  context.strokeStyle = ink;
  context.fillStyle = ink;
  context.lineWidth = stroke.size;
  context.lineCap = "round";
  context.lineJoin = "round";
  if (rest.length === 0) {
    context.beginPath();
    context.arc(first.x, first.y, stroke.size / 2, 0, Math.PI * 2);
    context.fill();
    return;
  }
  context.beginPath();
  context.moveTo(first.x, first.y);
  for (const point of rest) context.lineTo(point.x, point.y);
  context.stroke();
}

/** hasPaint は塗りの筆跡が 1 つでもあるかを返す（消しで覆われたかは見ない）。 */
export function hasPaint(strokes: readonly Stroke[]): boolean {
  return strokes.some((stroke) => stroke.mode === "paint" && stroke.points.length > 0);
}

/**
 * buildMask はコマの大きさの PNG を作る。塗った画素が不透明な白、残りが不透明な黒である。
 * 塗りが無い（消しで全部消えた場合も含む）ときは `undefined` を返し、マスクを送らない。
 */
export async function buildMask(
  strokes: readonly Stroke[],
  width: number,
  height: number,
): Promise<Blob | undefined> {
  if (!hasPaint(strokes)) return undefined;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (context === null) return undefined;
  context.fillStyle = "black";
  context.fillRect(0, 0, width, height);
  for (const stroke of strokes) drawStroke(context, stroke, "white", "black");
  // 消しで塗りが全部消えたなら送らない。縁のぼかしで生じた中間の色は白か黒に寄せる。
  const image = context.getImageData(0, 0, width, height);
  const data = image.data;
  let painted = false;
  for (let index = 0; index < data.length; index += 4) {
    const value = (data[index] ?? 0) >= 128 ? 255 : 0;
    if (value === 255) painted = true;
    data[index] = value;
    data[index + 1] = value;
    data[index + 2] = value;
    data[index + 3] = 255;
  }
  if (!painted) return undefined;
  context.putImageData(image, 0, 0);
  return new Promise((resolve) => {
    canvas.toBlob((blob) => resolve(blob ?? undefined), "image/png");
  });
}
