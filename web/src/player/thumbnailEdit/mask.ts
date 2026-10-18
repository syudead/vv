// Keep strokes in frame pixels so resizing preserves their positions.

export interface Stroke {
  mode: "paint" | "erase";
  /** Brush diameter in frame pixels. */
  size: number;
  /** Frame-pixel points; a single point draws a circular dot. */
  points: { x: number; y: number }[];
}

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

/** Erase with destination-out for display, or a supplied color for exported masks. */
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

/** Checks for paint strokes without accounting for later erasure. */
export function hasPaint(strokes: readonly Stroke[]): boolean {
  return strokes.some((stroke) => stroke.mode === "paint" && stroke.points.length > 0);
}

/** Export opaque white paint over black; return undefined when no paint survives erasure. */
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
  // Skip fully erased masks and threshold antialiased edge pixels to black or white.
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
