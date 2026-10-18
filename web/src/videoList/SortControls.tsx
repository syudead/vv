import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarArrowDown,
  CalendarClock,
  CalendarHeart,
  ChevronDown,
  Dices,
  FileClock,
  HardDrive,
  History,
  type LucideIcon,
  Shuffle,
  Timer,
} from "lucide-react";

import { type ReactNode, useId } from "react";

import type { VideoSort } from "../api/client";
import { useAudience } from "../auth/audience";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { FieldLegend, FieldSet } from "../ui/shadcn/field";
import { Label } from "../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../ui/shadcn/radio-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import IconToggleGroup from "./IconToggleGroup";
import {
  directionLabel,
  directionToggleLabel,
  ownerOnlySortKinds,
  type SortDirection,
  type SortKind,
  sortDirection,
  sortKindLabel,
  sortKindOf,
  sortKinds,
  withDirection,
} from "./listCriteria";

export const sortIcons: Record<SortKind, LucideIcon> = {
  added: CalendarArrowDown,
  modified: CalendarClock,
  created: FileClock,
  title: ArrowDownAZ,
  duration: Timer,
  size: HardDrive,
  played: History,
  favorited: CalendarHeart,
  random: Shuffle,
};

/** Each sort kind supplies its default direction. */
export const sortOptions = sortKinds.map((info) => ({
  kind: info.kind,
  value: info.initial,
  icon: sortIcons[info.kind],
}));

/** Guests cannot sort by playback or favorite dates because those are owner data. */
function useSortOptions(): typeof sortOptions {
  const owner = useAudience() === "owner";
  return owner
    ? sortOptions
    : sortOptions.filter((option) => !ownerOnlySortKinds.includes(option.kind));
}

export interface SortControlProps {
  sort: VideoSort;
  /** The caller creates a fresh seed when random is selected. */
  onSortChange: (value: VideoSort) => void;
  onShuffle: () => void;
  disabled?: boolean;
}

export function SortMenu({ sort, onSortChange, onShuffle, disabled }: SortControlProps) {
  const options = useSortOptions();
  const active = sortKindOf(sort);
  const direction = sortDirection(sort);
  const toggleLabel =
    direction === undefined ? t.list.sort.shuffle : directionToggleLabel(sort);

  return (
    <SortMenuView
      label={sortKindLabel(active.kind)}
      currentLabel={t.list.sort.current(sortKindLabel(active.kind))}
      heading={t.list.sort.heading}
      value={active.initial}
      options={options.map((option) => ({
        value: option.value,
        label: sortKindLabel(option.kind),
        icon: option.icon,
      }))}
      onValueChange={(value) => {
        const next = sortKindOf(value);
        if (next.kind !== active.kind) onSortChange(next.initial);
      }}
      toggle={{
        label: toggleLabel,
        icon:
          direction === undefined ? (
            <Dices />
          ) : direction === "desc" ? (
            <ArrowDownWideNarrow />
          ) : (
            <ArrowUpNarrowWide />
          ),
        onClick: () => {
          if (direction === undefined) onShuffle();
          else onSortChange(withDirection(sort, direction === "asc" ? "desc" : "asc"));
        },
      }}
      disabled={disabled}
    />
  );
}

export function CompactSortControls({
  sort,
  onSortChange,
  onShuffle,
  disabled,
  name,
}: SortControlProps & {
  /** Radio group name must be unique on the page. */ name: string;
}) {
  const options = useSortOptions();
  const active = sortKindOf(sort);
  const direction = sortDirection(sort);

  return (
    <CompactSortView
      name={name}
      heading={t.list.sort.heading}
      value={active.initial}
      options={options.map((option) => ({
        value: option.value,
        label: sortKindLabel(option.kind),
        icon: option.icon,
      }))}
      onValueChange={(value) => onSortChange(value)}
      disabled={disabled}
    >
      {direction === undefined ? (
        <Button variant="outline" size="sm" onClick={onShuffle} disabled={disabled}>
          <Dices aria-hidden="true" />
          {t.list.sort.shuffle}
        </Button>
      ) : (
        <IconToggleGroup<SortDirection>
          label={t.list.sort.direction}
          value={direction}
          onValueChange={(next) => onSortChange(withDirection(sort, next))}
          options={[
            {
              value: "desc",
              label: directionLabel(sort, "desc"),
              icon: <ArrowDownWideNarrow aria-hidden="true" />,
            },
            {
              value: "asc",
              label: directionLabel(sort, "asc"),
              icon: <ArrowUpNarrowWide aria-hidden="true" />,
            },
          ]}
        />
      )}
    </CompactSortView>
  );
}

export interface SortOption<T extends string> {
  value: T;
  label: UiText;
  icon: LucideIcon;
}

/** Without a direction toggle, render a standalone rounded menu button. */
export function SortMenuView<T extends string>({
  label,
  currentLabel,
  heading,
  value,
  options,
  onValueChange,
  toggle,
  disabled,
}: {
  label: UiText;
  currentLabel: UiText;
  heading: UiText;
  value: T;
  options: readonly SortOption<T>[];
  onValueChange: (value: T) => void;
  toggle?: { label: UiText; icon: ReactNode; onClick: () => void };
  disabled?: boolean;
}) {
  return (
    <div className="flex">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            aria-label={currentLabel}
            disabled={disabled}
            className={cn(toggle !== undefined && "rounded-r-none")}
          >
            {label}
            <ChevronDown aria-hidden="true" className="text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>{heading}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={value}
            onValueChange={(next) => onValueChange(next as T)}
          >
            {options.map((option) => (
              <DropdownMenuRadioItem key={option.value} value={option.value}>
                <option.icon aria-hidden="true" />
                {option.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {toggle !== undefined && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={toggle.label}
              disabled={disabled}
              // Join the controls with only one border between them.
              className="rounded-l-none border-l-0"
              onClick={toggle.onClick}
            >
              {toggle.icon}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{toggle.label}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

export function CompactSortView<T extends string>({
  name,
  heading,
  value,
  options,
  onValueChange,
  disabled,
  children,
}: {
  /** Radio group name must be unique on the page. */
  name: string;
  heading: UiText;
  value: T;
  options: readonly SortOption<T>[];
  onValueChange: (value: T) => void;
  disabled?: boolean;
  children?: ReactNode;
}) {
  const idPrefix = useId();
  return (
    <FieldSet disabled={disabled} className="gap-3">
      <FieldLegend variant="label">{heading}</FieldLegend>
      <RadioGroup
        name={name}
        value={value}
        onValueChange={(next) => onValueChange(next as T)}
        disabled={disabled}
        className="grid-cols-2 gap-2"
      >
        {options.map((option) => {
          const id = `${idPrefix}-${option.value}`;
          return (
            <div key={option.value} className="flex min-w-0 items-center gap-2">
              <RadioGroupItem id={id} value={option.value} />
              <Label htmlFor={id} className="min-w-0 font-normal">
                <option.icon aria-hidden="true" className="size-4 shrink-0" />
                {/* Wrap labels to two lines instead of truncating on narrow screens. */}
                <span>{option.label}</span>
              </Label>
            </div>
          );
        })}
      </RadioGroup>
      {children}
    </FieldSet>
  );
}
