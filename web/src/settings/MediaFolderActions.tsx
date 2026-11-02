import { Pencil, RotateCcw, Trash2 } from "lucide-react";

import { t } from "../i18n";
import { RowMoreMenu } from "../tags/TagRow";
import { DropdownMenuItem, DropdownMenuSeparator } from "../ui/shadcn/dropdown-menu";

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
  return (
    <RowMoreMenu
      buttonRef={null}
      label={text.actions(path)}
      disabled={disabled}
      describedBy={describedBy}
      className="shrink-0"
    >
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
    </RowMoreMenu>
  );
}
