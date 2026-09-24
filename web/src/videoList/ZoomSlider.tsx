import * as Slider from "@radix-ui/react-slider";

import { cn } from "../lib/cn";
import type { Zoom } from "../preferences/viewPreferences";

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
    <Slider.Root
      value={[zoom]}
      min={0}
      max={3}
      step={1}
      onValueChange={([next]) => {
        if (next !== undefined) onZoomChange(next as Zoom);
      }}
      className={cn("relative flex h-9 touch-none items-center select-none", className)}
    >
      <Slider.Track className="relative h-1 grow rounded-full bg-border-strong">
        <Slider.Range className="absolute h-full rounded-full bg-accent" />
      </Slider.Track>
      <Slider.Thumb
        aria-label="カードの大きさ"
        className="block size-4 rounded-full bg-fg shadow-card transition-transform hover:scale-110"
      />
    </Slider.Root>
  );
}
