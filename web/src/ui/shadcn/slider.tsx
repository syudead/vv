import { Slider as SliderPrimitive } from "radix-ui";
import { type ComponentProps, useMemo } from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の Slider（radix）。上流に加えて aria-label をつまみへ渡す（つまみが
// フォーカスを受けるので、読み上げ名はつまみに要る）。
// 規則は web/registry/rules/components.md の Slider。
function Slider({
  className,
  defaultValue,
  value,
  min = 0,
  max = 100,
  "aria-label": ariaLabel,
  ...props
}: ComponentProps<typeof SliderPrimitive.Root>) {
  const values = useMemo(
    () =>
      Array.isArray(value)
        ? value
        : Array.isArray(defaultValue)
          ? defaultValue
          : [min, max],
    [value, defaultValue, min, max],
  );

  return (
    <SliderPrimitive.Root
      data-slot="slider"
      defaultValue={defaultValue}
      value={value}
      min={min}
      max={max}
      className={cn(
        "relative flex w-full touch-none items-center select-none data-[disabled]:opacity-50 data-[orientation=vertical]:h-full data-[orientation=vertical]:min-h-16 data-[orientation=vertical]:w-auto data-[orientation=vertical]:flex-col",
        className,
      )}
      {...props}
    >
      <SliderPrimitive.Track
        data-slot="slider-track"
        className="relative grow overflow-hidden rounded-full bg-input data-[orientation=horizontal]:h-1 data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-1"
      >
        <SliderPrimitive.Range
          data-slot="slider-range"
          className="absolute bg-primary data-[orientation=horizontal]:h-full data-[orientation=vertical]:w-full"
        />
      </SliderPrimitive.Track>
      {Array.from({ length: values.length }, (_, index) => (
        <SliderPrimitive.Thumb
          data-slot="slider-thumb"
          key={index}
          aria-label={ariaLabel}
          className="block size-4 shrink-0 rounded-full border border-primary bg-foreground transition-colors hover:border-primary-hover hover:bg-primary-hover active:bg-primary-active disabled:pointer-events-none disabled:opacity-50"
        />
      ))}
    </SliderPrimitive.Root>
  );
}

export { Slider };
