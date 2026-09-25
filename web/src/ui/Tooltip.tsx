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
  container,
}: {
  content: ReactNode;
  children: ReactElement;
  side?: "top" | "bottom" | "left" | "right";
  /** 吹き出しを描く先。無ければ body。全画面の要素の中から出すときに渡す。 */
  container?: HTMLElement | null;
}) {
  return (
    <RadixTooltip.Root>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal container={container ?? undefined}>
        <RadixTooltip.Content
          side={side}
          sideOffset={6}
          className="z-50 rounded-sm bg-navbar px-2 py-1 text-xs text-fg shadow-elevated animate-fade-in select-none"
        >
          {content}
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  );
}
