import { Slot } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の skeleton（radix-nova）を vv のトークンで着せたもの。読み込み中に、出来上がりの
// 形を先に見せる（web/registry/rules/components.md「Skeleton」）。動きは上流の pulse でなく
// vv の shimmer（tokens.css の bg-shimmer と animate-shimmer）。p の中のような句の中では
// asChild で span を渡し、div を置かない。

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
