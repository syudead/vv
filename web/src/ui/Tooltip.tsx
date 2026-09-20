import * as RadixTooltip from "@radix-ui/react-tooltip";
import type { ReactElement, ReactNode } from "react";

export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <RadixTooltip.Provider delayDuration={400} skipDelayDuration={200}>
      {children}
    </RadixTooltip.Provider>
  );
}

export default function Tooltip({
  content,
  children,
  side = "bottom",
}: {
  content: ReactNode;
  children: ReactElement;
  side?: "top" | "bottom" | "left" | "right";
}) {
  return (
    <RadixTooltip.Root>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content
          side={side}
          sideOffset={6}
          className="z-50 rounded-sm bg-fg px-2 py-1 text-xs font-medium text-bg shadow-elevated animate-fade-in select-none"
        >
          {content}
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  );
}
