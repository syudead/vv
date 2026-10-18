import { Shapes } from "lucide-react";
import type { Ref } from "react";

import type { TagKind } from "../api/tagKinds";
import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";

export type KindChoice = TagKind | "none";

/** Mixed selections have no checked kind; all choices remain one radio group across the visual separator. */
export default function TagSetKindMenu({
  kinds,
  checked,
  disabled,
  pending,
  title,
  describedBy,
  onChoose,
  buttonRef,
}: {
  kinds: readonly TagKind[] | undefined;
  checked: string | undefined;
  disabled: boolean;
  pending: boolean;
  title: UiText | undefined;
  describedBy: string | undefined;
  onChoose: (choice: KindChoice) => void;
  buttonRef?: Ref<HTMLButtonElement>;
}) {
  const list = kinds ?? [];
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={buttonRef}
          variant="ghost"
          size="sm"
          disabled={disabled}
          title={title}
          aria-describedby={describedBy}
        >
          {pending ? <Spinner aria-hidden="true" /> : <Shapes aria-hidden="true" />}
          <span>{t.tags.selection.setKind}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {kinds !== undefined && list.length === 0 && (
          <DropdownMenuLabel className="max-w-menu font-normal">
            {t.tags.selection.noKinds}
          </DropdownMenuLabel>
        )}
        {}
        <DropdownMenuRadioGroup value={checked ?? ""}>
          {list.map((kind) => (
            <DropdownMenuRadioItem
              key={kind.id}
              value={String(kind.id)}
              onSelect={() => onChoose(kind)}
            >
              <span title={kind.name} className="max-w-chip-label min-w-0 truncate">
                {kind.name}
              </span>
            </DropdownMenuRadioItem>
          ))}
          {list.length > 0 && <DropdownMenuSeparator />}
          <DropdownMenuRadioItem value="none" onSelect={() => onChoose("none")}>
            {t.tags.kindFilter.none}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
