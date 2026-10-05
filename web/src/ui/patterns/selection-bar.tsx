import { X } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";

// 選択バー（区画）。選んだ数、選択の解除と、選んだものへの一括の操作を出す。何かを
// 選んでいる間だけ置く。置き場は 2 つ: bottom（既定）はページの下端に浮かせて貼り付け、
// header は管理表ページの見出しの行の代わりに、貼り付いた帯の中に置く。規則は
// web/registry/rules/patterns.md の Sections。

export interface SelectionBarProps {
  /** 選んだ数（「2 selected」）。 */
  count: ReactNode;
  /** 選択を解除するボタンの名前（「Clear selection」）。 */
  clearLabel: string;
  onClear: () => void;
  /** 選択を解除するボタンを押せなくする（一括の操作の送信中など）。 */
  clearDisabled?: boolean;
  /** 一括の操作（ghost の sm のボタン）。 */
  children: ReactNode;
  /** 置き場。既定は bottom。header は管理表ページの見出しの行の代わり。 */
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
  return (
    <div
      data-slot="selection-bar"
      data-placement={placement}
      className={cn(
        "flex items-center gap-2",
        placement === "bottom"
          ? "sticky bottom-3 z-10 mx-auto h-selection-bar w-full max-w-4xl animate-slide-up rounded-lg border border-border bg-popover px-2 text-popover-foreground shadow-elevated motion-reduce:animate-none"
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
        className="ml-auto flex min-w-0 items-center gap-1 overflow-x-auto"
      >
        {children}
      </div>
    </div>
  );
}
