import {
  type KeyboardEvent,
  type PointerEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import FittedBox from "./FittedBox";
import { drawStroke, type Stroke } from "./mask";

/**
 * PaintStage は編集画面の段で、コマを全体が見えるいちばん大きな大きさで出し、同じ大きさの
 * 塗りの層を重ねる（ui-design「Compose」）。マウス・ペン・指 1 本で塗り、塗りの下で段は
 * スクロールも拡大もしない（touch-none）。ポイントできる入力ではブラシの輪郭を出す。
 * Ctrl+Z・⌘+Z は層にフォーカスがあるとき最後の筆跡を取り消す。`locked` の間は塗れず、
 * 塗りは見えたまま残る。
 */
export default function PaintStage({
  frameUrl,
  frameWidth,
  frameHeight,
  strokes,
  mode,
  brushSize,
  locked,
  onStroke,
  onUndo,
}: {
  frameUrl: string;
  frameWidth: number;
  frameHeight: number;
  strokes: readonly Stroke[];
  mode: Stroke["mode"];
  /** ブラシの直径（コマの画素）。 */
  brushSize: number;
  locked: boolean;
  onStroke: (stroke: Stroke) => void;
  onUndo: () => void;
}) {
  const canvas = useRef<HTMLCanvasElement | null>(null);
  const current = useRef<{ pointerId: number; stroke: Stroke } | null>(null);
  const [outline, setOutline] = useState<{ x: number; y: number; size: number } | null>(
    null,
  );

  const paintColor = useCallback(() => {
    const element = canvas.current;
    if (element === null) return "white";
    // 塗りの色は primary（canvas の text-primary）で、層ごと opacity-50 で重ねる。
    return getComputedStyle(element).color || "white";
  }, []);

  const redraw = useCallback(() => {
    const element = canvas.current;
    const context = element?.getContext("2d") ?? null;
    if (element === null || context === null) return;
    context.globalCompositeOperation = "source-over";
    context.clearRect(0, 0, element.width, element.height);
    const color = paintColor();
    for (const stroke of strokes) drawStroke(context, stroke, color, null);
    if (current.current !== null)
      drawStroke(context, current.current.stroke, color, null);
  }, [paintColor, strokes]);

  useLayoutEffect(redraw, [redraw, frameWidth, frameHeight]);

  // 筆跡の途中で層が固まったら（実行が始まった、セッションが終わった）、その筆跡は捨てる。
  useEffect(() => {
    if (!locked) return;
    current.current = null;
    setOutline(null);
    redraw();
  }, [locked, redraw]);

  const framePoint = (event: PointerEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    return {
      x: ((event.clientX - rect.left) / rect.width) * frameWidth,
      y: ((event.clientY - rect.top) / rect.height) * frameHeight,
      scale: rect.width / frameWidth,
      cssX: event.clientX - rect.left,
      cssY: event.clientY - rect.top,
    };
  };

  const showOutline = (event: PointerEvent<HTMLCanvasElement>) => {
    if (locked || event.pointerType === "touch") {
      setOutline(null);
      return;
    }
    const point = framePoint(event);
    if (point === null) return;
    setOutline({ x: point.cssX, y: point.cssY, size: brushSize * point.scale });
  };

  const onPointerDown = (event: PointerEvent<HTMLCanvasElement>) => {
    if (locked || current.current !== null) return;
    if (event.pointerType === "mouse" && event.button !== 0) return;
    const point = framePoint(event);
    if (point === null) return;
    event.preventDefault();
    event.currentTarget.focus({ preventScroll: true });
    event.currentTarget.setPointerCapture?.(event.pointerId);
    current.current = {
      pointerId: event.pointerId,
      stroke: { mode, size: brushSize, points: [{ x: point.x, y: point.y }] },
    };
    redraw();
  };

  const onPointerMove = (event: PointerEvent<HTMLCanvasElement>) => {
    showOutline(event);
    const active = current.current;
    if (active?.pointerId !== event.pointerId) return;
    const point = framePoint(event);
    if (point === null) return;
    active.stroke.points.push({ x: point.x, y: point.y });
    redraw();
  };

  const finish = (event: PointerEvent<HTMLCanvasElement>) => {
    const active = current.current;
    if (active?.pointerId !== event.pointerId) return;
    current.current = null;
    onStroke(active.stroke);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLCanvasElement>) => {
    if (locked) return;
    if (
      (event.ctrlKey || event.metaKey) &&
      !event.shiftKey &&
      event.key.toLowerCase() === "z"
    ) {
      event.preventDefault();
      onUndo();
    }
  };

  return (
    <FittedBox aspect={frameWidth / frameHeight}>
      <img
        src={frameUrl}
        alt=""
        draggable={false}
        className="absolute inset-0 size-full object-contain select-none"
      />
      <canvas
        ref={canvas}
        width={frameWidth}
        height={frameHeight}
        role="img"
        aria-label={t.player.thumbnailEdit.paintedArea}
        aria-disabled={locked || undefined}
        tabIndex={locked ? -1 : 0}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={finish}
        onPointerCancel={finish}
        onPointerLeave={() => setOutline(null)}
        onKeyDown={onKeyDown}
        className={cn(
          "absolute inset-0 size-full touch-none text-primary opacity-50",
          locked ? "cursor-default" : "cursor-crosshair",
        )}
      />
      {outline !== null && (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -translate-x-1/2 -translate-y-1/2 rounded-full border border-foreground"
          style={{
            left: outline.x,
            top: outline.y,
            width: outline.size,
            height: outline.size,
          }}
        />
      )}
    </FittedBox>
  );
}
