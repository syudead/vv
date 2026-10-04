import { Pause, Play } from "lucide-react";

import { t } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";

/**
 * TouchControls はタッチの端末だけに出す、プレイヤー中央の大きな再生/一時停止である
 * （要件 8）。秒数送りのボタンは置かない。
 *
 * 出し分けは CSS の `pointer: coarse` だけで行う。見せる時期は操作バーと同じで、
 * `visible` が偽の間は見えなくし、押せなくする。指で押す的なので、`Button` の `icon` を
 * 丸く大きく（`size-16`）し、アイコンも大きくする（`size-6`）。
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
            // タップで残るフォーカスでは見せない（focus-within だと再生中も消えなくなる）。
            "pointer-events-none opacity-0 has-[button:focus-visible]:opacity-100 [&_button]:pointer-events-none",
      )}
    >
      <Button
        variant="secondary"
        size="icon"
        aria-label={playing ? t.player.controls.pause : t.player.controls.play}
        onClick={onToggle}
        className="pointer-events-auto size-16 rounded-full shadow-elevated [&_svg]:size-6"
      >
        {playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
      </Button>
    </div>
  );
}
