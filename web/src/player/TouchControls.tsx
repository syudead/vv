import { Pause, Play } from "lucide-react";

import { cn } from "../lib/cn";

/**
 * TouchControls はタッチの端末だけに出す、プレイヤー中央の大きな再生/一時停止である
 * （要件 8）。秒数送りのボタンは置かない。
 *
 * 出し分けは CSS の `pointer: coarse` だけで行う。見せる時期は操作バーと同じで、
 * `visible` が偽の間は見えなくし、押せなくする。
 */
export default function TouchControls({
  playing,
  visible,
  onToggle,
}: {
  playing: boolean;
  visible: boolean;
  onToggle: () => void;
}) {
  return (
    <div
      data-touch-controls=""
      className={cn(
        "hidden w-full items-center justify-center transition-opacity duration-150 motion-reduce:transition-none [@media(pointer:coarse)]:flex",
        visible
          ? "opacity-100"
          : // 見えない間は押せなくする。Tab でフォーカスが来たら見せ、輪郭を隠さない。
            "pointer-events-none opacity-0 focus-within:opacity-100 [&_button]:pointer-events-none",
      )}
    >
      <button
        type="button"
        aria-label={playing ? "一時停止" : "再生"}
        onClick={onToggle}
        className="pointer-events-auto flex size-15 items-center justify-center rounded-full bg-overlay text-fg"
      >
        {playing ? (
          <Pause className="size-7" aria-hidden="true" />
        ) : (
          <Play className="size-7" aria-hidden="true" />
        )}
      </button>
    </div>
  );
}
