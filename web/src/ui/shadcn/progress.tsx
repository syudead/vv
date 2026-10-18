import { Progress as ProgressPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/cn";

// Adapted from shadcn/ui radix-nova with shared tokens.
// （web/registry/rules/components.md「Progress」）。
//
// Scale against max rather than upstream's fixed 100; null renders indeterminate progress.

function Progress({
  className,
  value,
  max = 100,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root>) {
  const percent = value == null || max <= 0 ? null : Math.min(100, (value / max) * 100);
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      value={value}
      max={max}
      className={cn(
        "relative flex h-1 w-full items-center overflow-x-hidden rounded-full bg-muted",
        className,
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={cn(
          "h-full bg-primary transition-transform motion-reduce:transition-none",
          percent === null
            ? "w-1/3 animate-pulse motion-reduce:animate-none"
            : "w-full flex-1",
        )}
        style={
          percent === null ? undefined : { transform: `translateX(-${100 - percent}%)` }
        }
      />
    </ProgressPrimitive.Root>
  );
}

export { Progress };
