import { t } from "../i18n";
import { cn } from "../lib/cn";
import type { Zoom } from "../preferences/viewPreferences";
import { Slider } from "../ui/slider";

/** ZoomSlider はカードの大きさ（4 段）を選ぶスライダーである。ライブラリとフォルダ画面で使う。 */
export default function ZoomSlider({
  zoom,
  onZoomChange,
  className,
}: {
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
  className?: string;
}) {
  return (
    <Slider
      value={[zoom]}
      min={0}
      max={3}
      step={1}
      aria-label={t.list.cardSize}
      onValueChange={([next]) => {
        if (next !== undefined) onZoomChange(next as Zoom);
      }}
      className={cn("h-8", className)}
    />
  );
}
