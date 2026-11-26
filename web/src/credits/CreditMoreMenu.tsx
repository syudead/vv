import { Ellipsis, Merge, Trash2 } from "lucide-react";

import type { CreditKind } from "../api/credits";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { kindTexts } from "./creditRows";

const text = t.credits.personPage;

/** A profile page's `More actions` menu: the merge, then `Delete` as its only destructive item. */
export function CreditMoreMenu({
  kind,
  onMerge,
  onDelete,
}: {
  kind: CreditKind;
  onMerge: () => void;
  onDelete: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={text.more}>
          <Ellipsis aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={onMerge}>
          <Merge aria-hidden="true" />
          {kindTexts(kind).mergeInto}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={onDelete}>
          <Trash2 aria-hidden="true" />
          {text.delete}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
