import { ArrowDown, ArrowUp, Ellipsis, Minus } from "lucide-react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import type { ShelfMoves } from "./shelfOrder";

/**
 * ShelfMenu は所有者のフォルダの棚の見出しの「More」である（docs/design-docs/home.md
 * 「Home」）。「Move up」「Move down」「Remove from home」はどれも押すとすぐ書く。最初の棚に
 * 「Move up」は無く、最後の棚に「Move down」は無い。
 */
export default function ShelfMenu({
  name,
  moves,
  disabled,
  onMove,
  onRemove,
}: {
  name: string;
  moves: ShelfMoves;
  disabled: boolean;
  onMove: (direction: -1 | 1) => void;
  onRemove: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t.home.shelfMore(name)}
          disabled={disabled}
        >
          <Ellipsis aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {moves.up && (
          <DropdownMenuItem onSelect={() => onMove(-1)}>
            <ArrowUp aria-hidden="true" />
            {t.home.moveUp}
          </DropdownMenuItem>
        )}
        {moves.down && (
          <DropdownMenuItem onSelect={() => onMove(1)}>
            <ArrowDown aria-hidden="true" />
            {t.home.moveDown}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onSelect={onRemove}>
          <Minus aria-hidden="true" />
          {t.home.removeFromHome}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
