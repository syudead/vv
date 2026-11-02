import { Ellipsis, Pencil, RotateCcw, Trash2 } from "lucide-react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * One menu for a local row's Change, Force rescan and Remove, so the path keeps the row's width.
 * A running scan or a pending change disables the trigger, and describedBy names the reason.
 */
export function MediaFolderActions({
  path,
  disabled,
  describedBy,
  onChange,
  onRescan,
  onRemove,
}: {
  path: string;
  disabled: boolean;
  describedBy?: string;
  onChange: () => void;
  onRescan: () => void;
  onRemove: () => void;
}) {
  const text = t.settings.mediaFolders;
  const label = text.actions(path);
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              aria-describedby={describedBy}
              disabled={disabled}
              className="shrink-0"
            >
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={onChange}>
          <Pencil aria-hidden="true" />
          {text.change}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onRescan}>
          <RotateCcw aria-hidden="true" />
          {text.forceRescan}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={onRemove}>
          <Trash2 aria-hidden="true" />
          {text.remove}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
