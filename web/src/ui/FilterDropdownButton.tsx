import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/ui/shadcn/dropdown-menu";

//
// See web/registry/rules/components.md, FilterDropdownButton.

export interface FilterDropdownButtonProps {
  name: ReactNode;
  value: ReactNode;
  active: boolean;
  /** Supply a radio group or checkbox menu items. */
  children: ReactNode;
  /** Menu alignment defaults to start. */
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
