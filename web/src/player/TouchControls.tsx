import { Pause, Play, RotateCcw, RotateCw } from "lucide-react";

import { cn } from "../lib/cn";

/**
 * TouchControls はタッチの端末だけに出す、プレイヤー中央の大きな操作である（要件 8）。
 *
 * 出し分けは CSS の `pointer: coarse` だけで行う。見せる時期は操作バーと同じで、
 * `visible` が偽の間は見えなくし、押せなくする。
 */
export default function TouchControls({
  playing,
  visible,
  onBack,
  onToggle,
  onForward,
}: {
  playing: boolean;
  visible: boolean;
  onBack: () => void;
  onToggle: () => void;
  onForward: () => void;
}) {
  const round =
    "pointer-events-auto flex items-center justify-center rounded-full bg-overlay text-fg";
  return (
    <div
      data-touch-controls=""
      className={cn(
        "hidden w-full items-center justify-center gap-7 transition-opacity duration-150 motion-reduce:transition-none [@media(pointer:coarse)]:flex",
        visible
          ? "opacity-100"
          : "pointer-events-none opacity-0 [&_button]:pointer-events-none",
      )}
    >
      <button
        type="button"
        aria-label="10 秒戻る"
        onClick={onBack}
        className={cn(round, "size-12")}
      >
        <RotateCcw className="size-6" aria-hidden="true" />
      </button>
      <button
        type="button"
        aria-label={playing ? "一時停止" : "再生"}
        onClick={onToggle}
        className={cn(round, "size-15")}
      >
        {playing ? (
          <Pause className="size-7" aria-hidden="true" />
        ) : (
          <Play className="size-7" aria-hidden="true" />
        )}
      </button>
      <button
        type="button"
        aria-label="10 秒進む"
        onClick={onForward}
        className={cn(round, "size-12")}
      >
        <RotateCw className="size-6" aria-hidden="true" />
      </button>
    </div>
  );
}
