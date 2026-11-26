import { Ellipsis } from "lucide-react";
import type { ReactNode, Ref } from "react";

import { Button } from "./shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "./shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "./shadcn/tooltip";

export interface TagRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
  synonymsButton: HTMLButtonElement | null;
  menuButton: HTMLButtonElement | null;
  actionsButton: HTMLButtonElement | null;
}

/** describedBy names why the trigger is disabled; className places it in the row's layout. */
export function RowMoreMenu({
  buttonRef,
  label,
  disabled,
  describedBy,
  className,
  children,
}: {
  buttonRef: Ref<HTMLButtonElement>;
  label: string;
  disabled: boolean;
  describedBy?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              ref={buttonRef}
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              aria-describedby={describedBy}
              disabled={disabled}
              className={className}
            >
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}
