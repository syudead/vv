import { Progress as ProgressPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の progress（radix-nova）を vv のトークンで着せたもの。取り込みや視聴の進み具合
// （web/registry/rules/components.md「Progress」）。
//
// 上流との違い: 帯の長さを `max` に対する割合で求める（上流は max を 100 と決めている）。
// `value` が null のときは不確定のバー（Radix の data-state="indeterminate"）で、帯の 3 分の 1 を
// 明滅させる。

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
