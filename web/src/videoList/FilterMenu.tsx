import { ListFilter } from "lucide-react";
import { type ReactNode, type Ref, useId, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import { formatNumber, t, type UiText } from "../i18n";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Checkbox } from "../ui/shadcn/checkbox";
import { Field, FieldContent, FieldDescription, FieldLabel } from "../ui/shadcn/field";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";

export interface FilterMenuProps {
  /** The applied value; the popover edits a draft of it. */
  favorite: boolean;
  /** Apply the drafted favorite together with the page's own drafted fields. */
  onApply: (favorite: boolean) => void;
  canClear: boolean;
  /** Clear conditions but retain sort. */
  onClear: () => void;
  disabled?: boolean;
  /** Page-specific fields drawn before `Favorites only`. */
  children?: ReactNode;
  /** Include custom child-row filters in the button count. */
  extraCount?: number;
  /** Called on each opening; pages reset the drafts of their own fields here. */
  onOpen?: () => void;
}

/**
 * Exclude search from the badge because it is visible in the input. Every list filter is owner
 * data, so guests get no Filter button rather than an empty popover.
 */
export default function FilterMenu({
  favorite,
  onApply,
  canClear,
  onClear,
  disabled,
  children,
  extraCount = 0,
  onOpen,
}: FilterMenuProps) {
  const owner = useAudience() === "owner";
  const [draftFavorite, setDraftFavorite] = useState(favorite);
  if (!owner) return null;
  const filterCount = (favorite ? 1 : 0) + extraCount;

  return (
    <FilterPopover
      count={filterCount}
      disabled={disabled}
      canClear={canClear}
      onClear={onClear}
      onOpen={() => {
        setDraftFavorite(favorite);
        onOpen?.();
      }}
      onApply={() => onApply(draftFavorite)}
    >
      {children}
      <FilterCheckbox
        checked={draftFavorite}
        onCheckedChange={setDraftFavorite}
        label={t.list.filter.favoritesOnly}
      />
    </FilterPopover>
  );
}

export interface FilterPopoverProps {
  count: number;
  disabled?: boolean;
  canClear: boolean;
  onClear: () => void;
  triggerRef?: Ref<HTMLButtonElement>;
  onOpen?: () => void;
  /** With it, the fields edit drafts that only this button applies; closing otherwise drops them. */
  onApply?: () => void;
  children: ReactNode;
}

export function FilterPopover({
  count,
  disabled,
  canClear,
  onClear,
  triggerRef,
  onOpen,
  onApply,
  children,
}: FilterPopoverProps) {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const shown = disabled ? 0 : count;

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) onOpen?.();
      }}
    >
      <PopoverTrigger asChild>
        <Button
          ref={triggerRef}
          variant="outline"
          size="sm"
          disabled={disabled}
          aria-label={shown > 0 ? t.list.filter.applied(shown) : t.list.filter.label}
        >
          <ListFilter aria-hidden="true" />
          <span className="hidden xl:inline">{t.list.filter.label}</span>
          {shown > 0 && <Badge variant="soft">{formatNumber(shown)}</Badge>}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="gap-4"
        ref={panelRef}
        aria-label={t.list.filter.label}
        // Focus the panel, not its first field: the tag picker opens its candidate list on focus.
        tabIndex={-1}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          panelRef.current?.focus();
        }}
      >
        {children}

        {(canClear || onApply !== undefined) && (
          <div className="flex gap-2">
            {canClear && (
              <Button
                variant="ghost"
                size="sm"
                className="flex-1"
                onClick={() => {
                  onClear();
                  // Radix restores focus to the filter button on close.
                  setOpen(false);
                }}
              >
                {t.list.filter.clear}
              </Button>
            )}
            {onApply !== undefined && (
              <Button
                size="sm"
                className="flex-1"
                onClick={() => {
                  onApply();
                  setOpen(false);
                }}
              >
                {t.list.filter.apply}
              </Button>
            )}
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}

export function FilterCheckbox({
  checked,
  onCheckedChange,
  label,
  hint,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: UiText;
  hint?: UiText;
}) {
  const id = useId();
  return (
    <Field orientation="horizontal">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(next) => onCheckedChange(next === true)}
      />
      <FieldContent>
        <FieldLabel htmlFor={id} className="font-normal">
          {label}
        </FieldLabel>
        {hint !== undefined && <FieldDescription>{hint}</FieldDescription>}
      </FieldContent>
    </Field>
  );
}
