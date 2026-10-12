import type { ComponentProps } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * ActionButton は情報の行の右端の、アイコンだけの操作である。お気に入り（Toggle の `sm`）と
 * 同じ大きさ（`icon-sm`）にそろえ、名前をツールチップでも出す。
 */
export default function ActionButton({
  label,
  className,
  children,
  ...props
}: ComponentProps<typeof Button> & { label: UiText }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          className={cn("text-muted-foreground", className)}
          {...props}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
