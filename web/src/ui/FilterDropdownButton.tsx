import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/ui/shadcn/dropdown-menu";

// 絞り込みのドロップダウンのボタン（vv の部品）。ツールバーに同じ形で並べ、絞り込みの名前と
// 今の値と山形の印を出し、押すと DropdownMenu を開く。既定の値でないときは primary-soft の面に
// primary の文字にして、開かずに今の値が読めるようにする
// （specs/055-activity/ui-design.md「Changes to the design system」）。
// 規則は web/registry/rules/components.md の FilterDropdownButton。

export interface FilterDropdownButtonProps {
  /** 絞り込みの名前（「Status」）。 */
  name: ReactNode;
  /** 今の値（「All」「Needs attention」「Import +2」）。 */
  value: ReactNode;
  /** 既定の値でない。 */
  active: boolean;
  /** メニューの中身: DropdownMenuRadioGroup か DropdownMenuCheckboxItem の並び。 */
  children: ReactNode;
  /** メニューの幅の揃え。既定は start。 */
  align?: "start" | "end";
}

export function FilterDropdownButton({
  name,
  value,
  active,
  children,
  align = "start",
}: FilterDropdownButtonProps) {
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          data-active={active ? "true" : undefined}
          className={cn(
            "shrink-0",
            active &&
              "border-primary bg-primary-soft text-primary hover:bg-primary-soft hover:text-primary",
          )}
        >
          <span>{name}</span>
          <span
            data-slot="filter-value"
            className={active ? "text-primary" : "text-muted-foreground"}
          >
            {value}
          </span>
          <ChevronDown aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align}>{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}
