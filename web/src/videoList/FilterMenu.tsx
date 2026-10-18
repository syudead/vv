import { ListFilter } from "lucide-react";
import { type ReactNode, type Ref, useId, useState } from "react";

import type { WatchFilter } from "../api/client";
import { useAudience } from "../auth/audience";
import { formatNumber, t, type UiText } from "../i18n";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Checkbox } from "../ui/shadcn/checkbox";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "../ui/shadcn/field";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { ToggleGroup, ToggleGroupItem } from "../ui/shadcn/toggle-group";
import { watchLabel, watchValues } from "./listSummary";

export interface FilterMenuProps {
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  canClear: boolean;
  /** Clear conditions but retain sort. */
  onClear: () => void;
  disabled?: boolean;
  children?: ReactNode;
  /** Include custom child-row filters in the button count. */
  extraCount?: number;
  /** Allows child rows to defer loading until the popover opens. */
  onOpen?: () => void;
}

/**
 * Exclude search from the badge because it is visible in the input; guests have only playability
 * filtering because watch state and favorites are owner data.
 */
export default function FilterMenu({
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  favorite,
  onFavoriteChange,
  canClear,
  onClear,
  disabled,
  children,
  extraCount = 0,
  onOpen,
}: FilterMenuProps) {
  const owner = useAudience() === "owner";
  const watchCount = owner && watch !== "all" ? 1 : 0;
  const favoriteCount = owner && favorite ? 1 : 0;
  const filterCount = watchCount + favoriteCount + (playable ? 1 : 0) + extraCount;

  return (
    <FilterPopover
      count={filterCount}
      disabled={disabled}
      canClear={canClear}
      onClear={onClear}
      onOpen={onOpen}
    >
      {owner && <WatchStatusField value={watch} onValueChange={onWatchChange} />}

      <div className="flex flex-col gap-2">
        {owner && (
          <FilterCheckbox
            checked={favorite}
            onCheckedChange={onFavoriteChange}
            label={t.list.filter.favoritesOnly}
          />
        )}
        <FilterCheckbox
          checked={playable}
          onCheckedChange={onPlayableChange}
          label={t.list.filter.playableOnly}
        />
      </div>
      {children}
    </FilterPopover>
  );
}

export function WatchStatusField({
  value,
  onValueChange,
}: {
  value: WatchFilter;
  onValueChange: (value: WatchFilter) => void;
}) {
  return (
    <FieldSet>
      <FieldLegend variant="label">{t.list.filter.watch}</FieldLegend>
      {/* Use radio semantics for mutually exclusive choices that apply immediately. */}
      <ToggleGroup
        type="single"
        size="sm"
        value={value}
        onValueChange={(next) => {
          if (next !== "") onValueChange(next as WatchFilter);
        }}
        aria-label={t.list.filter.watch}
        className="grid w-full grid-cols-2 gap-1"
      >
        {watchValues.map((option) => (
          <ToggleGroupItem key={option} value={option} className="rounded-md">
            {watchLabel(option)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </FieldSet>
  );
}

export interface FilterPopoverProps {
  count: number;
  disabled?: boolean;
  canClear: boolean;
  onClear: () => void;
  triggerRef?: Ref<HTMLButtonElement>;
  onOpen?: () => void;
  children: ReactNode;
}

export function FilterPopover({
  count,
  disabled,
  canClear,
  onClear,
  triggerRef,
  onOpen,
  children,
}: FilterPopoverProps) {
  const [open, setOpen] = useState(false);
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
      <PopoverContent align="start" className="gap-4">
        {children}

        {canClear && (
          <Button
            variant="ghost"
            size="sm"
            className="w-full"
            onClick={() => {
              onClear();
              // Radix restores focus to the filter button on close.
              setOpen(false);
            }}
          >
            {t.list.filter.clear}
          </Button>
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
