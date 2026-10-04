import { Tooltip as TooltipPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の tooltip（radix-nova）を vv のトークンで着せたもの
// （web/registry/rules/components.md「Tooltip」）。TooltipProvider はアプリに 1 つ置く。

function TooltipProvider({
  delayDuration = 400,
  skipDelayDuration = 200,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delayDuration={delayDuration}
      skipDelayDuration={skipDelayDuration}
      {...props}
    />
  );
}

function Tooltip({ ...props }: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />;
}

function TooltipTrigger({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

function TooltipContent({
  className,
  sideOffset = 4,
  container,
  children,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content> & {
  /** 描く先。全画面の要素の中から出すときに渡す。既定は body。 */
  container?: HTMLElement | null;
}) {
  return (
    <TooltipPrimitive.Portal container={container ?? undefined}>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          "z-50 inline-flex w-fit max-w-xs origin-(--radix-tooltip-content-transform-origin) items-center gap-1.5 rounded-md bg-foreground px-2 py-1 text-xs text-background select-none data-[state=delayed-open]:animate-fade-in data-[state=instant-open]:animate-fade-in motion-reduce:animate-none has-data-[slot=kbd]:pr-1",
          className,
        )}
        {...props}
      >
        {children}
        <TooltipPrimitive.Arrow className="fill-foreground" />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  );
}

export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
