import { X } from "lucide-react";
import { type ReactNode, type RefObject, useLayoutEffect, useRef } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";

// Keep action names visible and wrap them for touch devices; mount only while selected. See web/registry/rules/patterns.md, Sections.

export interface SelectionBarProps {
  count: ReactNode;
  clearLabel: string;
  onClear: () => void;
  clearDisabled?: boolean;
  children: ReactNode;
  placement?: "bottom" | "header";
}

export function SelectionBar({
  count,
  clearLabel,
  onClear,
  clearDisabled = false,
  children,
  placement = "bottom",
}: SelectionBarProps) {
  const ref = useRef<HTMLDivElement>(null);
  useSelectionBarHeight(ref, placement === "bottom");
  return (
    <div
      ref={ref}
      data-slot="selection-bar"
      data-placement={placement}
      className={cn(
        "flex items-center gap-2",
        // Later DOM order at z-30 keeps the bottom bar above card checkboxes scrolling beneath it.
        placement === "bottom"
          ? "sticky bottom-3 z-30 mx-auto min-h-selection-bar w-full max-w-4xl animate-slide-up flex-wrap rounded-lg border border-border bg-popover px-2 py-1 text-popover-foreground shadow-elevated motion-reduce:animate-none"
          : "min-h-8",
      )}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={clearLabel}
            disabled={clearDisabled}
            onClick={onClear}
          >
            <X aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{clearLabel}</TooltipContent>
      </Tooltip>
      <span
        data-slot="selection-bar-count"
        aria-live="polite"
        className="text-sm font-medium whitespace-nowrap tabular-nums"
      >
        {count}
      </span>
      <div
        data-slot="selection-bar-actions"
        className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1"
      >
        {children}
      </div>
    </div>
  );
}

/**
 * Publish the bottom bar's height as `--selection-bar-height` on the document, so notifications
 * placed above it clear it when its actions wrap onto more rows.
 */
function useSelectionBarHeight(ref: RefObject<HTMLDivElement | null>, enabled: boolean) {
  useLayoutEffect(() => {
    const bar = ref.current;
    if (!enabled || bar === null || typeof ResizeObserver !== "function") return;
    const root = document.documentElement;
    const publish = () =>
      root.style.setProperty("--selection-bar-height", `${String(bar.offsetHeight)}px`);
    publish();
    const observer = new ResizeObserver(publish);
    observer.observe(bar);
    return () => {
      observer.disconnect();
      root.style.removeProperty("--selection-bar-height");
    };
  }, [ref, enabled]);
}
