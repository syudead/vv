import { WifiLow, X } from "lucide-react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * StallWarning は、回線の遅さで再生が途切れているときにプレイヤーの左上に出す、知らせる
 * だけの帯である（specs/027-playback-quality/ui-design.md「Stall warning」、research.md R-7）。
 *
 * 状態表示の入れ物（`data-overlay-layer`）とは別の層で、映像より上・状態表示と操作バーより
 * 下に置く。× 以外は下の操作（再生バー・映像・中央の操作）へ通す。画質を変える操作は
 * 置かない（親 Issue 要件 10）。知らせるだけなので、読み上げは `alert` でなく `status` にする。
 */
export default function StallWarning({
  onDismiss,
  container,
}: {
  onDismiss: () => void;
  /** 全画面にしている入れ物。× のツールチップはその中に描かないと全画面の間に見えない。 */
  container: HTMLElement | null;
}) {
  const text = t.player.stallWarning;
  return (
    <div
      data-stall-warning=""
      className="pointer-events-none absolute inset-x-2 top-2 z-5 flex sm:inset-x-3 sm:top-3"
    >
      <div
        role="status"
        className="flex max-w-full items-center gap-2 rounded-md bg-navbar px-3 py-1.5 text-xs text-foreground shadow-elevated animate-fade-in motion-reduce:animate-none"
      >
        <WifiLow className="size-4 shrink-0 text-warning" aria-hidden="true" />
        <span className="min-w-0">{text.message}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={text.dismiss}
              onClick={onDismiss}
              className="pointer-events-auto -mr-1.5 size-6 rounded-sm text-muted-foreground [&_svg]:size-3"
            >
              <X aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent container={container}>{text.dismiss}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
