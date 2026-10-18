import { Slot } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/cn";

// Adapted from shadcn/ui radix-nova with shimmer instead of pulse. Use asChild with span inside phrasing content such as p.

function Skeleton({
  className,
  asChild = false,
  ...props
}: React.ComponentProps<"div"> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "div";
  return (
    <Comp
      data-slot="skeleton"
      aria-hidden="true"
      className={cn(
        "rounded-md bg-card bg-shimmer animate-shimmer motion-reduce:animate-none",
        className,
      )}
      {...props}
    />
  );
}

export { Skeleton };
