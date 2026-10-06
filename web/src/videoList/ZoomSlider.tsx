import { t } from "../i18n";
import { cn } from "../lib/cn";
import type { Zoom } from "../preferences/viewPreferences";
import { Slider } from "../ui/shadcn/slider";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * ZoomSlider はカードの大きさ（4 段）を選ぶスライダーである。ライブラリとフォルダ画面で使う。
 * tooltip はトップバーに名前なしで置くときに、名前（「Card size」）をツールチップで添える。
 */
export default function ZoomSlider({
  zoom,
  onZoomChange,
  className,
  tooltip = false,
}: {
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
  className?: string;
  tooltip?: boolean;
}) {
  const slider = (
    <Slider
      aria-label={t.list.cardSize}
      value={[zoom]}
      min={0}
      max={3}
      step={1}
      onValueChange={([next]) => {
        if (next !== undefined) onZoomChange(next as Zoom);
      }}
      className={cn(tooltip ? "h-8" : className)}
    />
  );
  if (!tooltip) return slider;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("flex", className)}>{slider}</span>
      </TooltipTrigger>
      <TooltipContent>{t.list.cardSize}</TooltipContent>
    </Tooltip>
  );
}
