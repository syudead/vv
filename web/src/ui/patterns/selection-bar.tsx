import { X } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";

// 選択バー（区画）。選んだ数、選択の解除と、選んだものへの一括の操作を、ページの下端に
// 貼り付けて出す。何かを選んでいる間だけ置く。規則は web/registry/rules/patterns.md の
// Sections。

export interface SelectionBarProps {
  /** 選んだ数（「2 selected」）。 */
  count: ReactNode;
  /** 選択を解除するボタンの名前（「Clear selection」）。 */
  clearLabel: string;
  onClear: () => void;
  /** 一括の操作（ghost の sm のボタン）。 */
  children: ReactNode;
}

export function SelectionBar({
  count,
  clearLabel,
  onClear,
  children,
}: SelectionBarProps) {
  return (
    <div
      data-slot="selection-bar"
      className="sticky bottom-3 z-10 mx-auto flex h-selection-bar w-full max-w-2xl animate-slide-up items-center gap-2 rounded-lg border border-border bg-popover px-2 text-popover-foreground shadow-elevated motion-reduce:animate-none"
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={clearLabel}
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
        className="ml-auto flex min-w-0 items-center gap-1 overflow-x-auto"
      >
        {children}
      </div>
    </div>
  );
}
