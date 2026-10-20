import { ArrowRight, Check, Plus } from "lucide-react";
import { Fragment, type ReactNode, useId, useState } from "react";

import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../ui/shadcn/collapsible";
import { Label } from "../../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../../ui/shadcn/radio-group";
import { Toggle } from "../../ui/shadcn/toggle";

// The parts a fetch comparison is built from, shared by the video's and the person's dialogs
// (docs/design-docs/work-info-plugins.md and person-plugins.md).

/** A part of a comparison with a count and, over more than one choice, Select all or Clear. */
export function CompareSection({
  title,
  note,
  count,
  all,
  locked,
  onAll,
  children,
}: {
  title: string;
  note?: string;
  count: number;
  all: boolean;
  locked: boolean;
  onAll: (on: boolean) => void;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-sm font-medium">
          {title} <span className="font-normal text-muted-foreground">{count}</span>
        </h3>
        {count > 1 && (
          <Button
            variant="link"
            size="sm"
            className="h-auto px-0 py-0"
            disabled={locked}
            onClick={() => onAll(!all)}
          >
            {all ? t.common.clearAll : t.common.selectAll}
          </Button>
        )}
      </div>
      {note !== undefined && <p className="text-xs text-muted-foreground">{note}</p>}
      <div className="flex flex-col divide-y divide-border border-y border-border">
        {children}
      </div>
    </section>
  );
}

interface RowsSection {
  all: boolean;
  onAll: (on: boolean) => void;
}

/** The rows that fill empty fields, then those that replace stored values, each a section when it has rows. */
export function ChangeSections<R>({
  fills,
  replaces,
  locked,
  keyOf,
  section,
  fill,
  replace,
}: {
  fills: readonly R[];
  replaces: readonly R[];
  locked: boolean;
  keyOf: (row: R) => string;
  section: (rows: readonly R[]) => RowsSection;
  fill: (row: R) => ReactNode;
  replace: (row: R) => ReactNode;
}) {
  const parts = [
    { rows: fills, title: t.common.fillsEmpty, note: undefined, render: fill },
    {
      rows: replaces,
      title: t.common.differsFromNow,
      note: t.common.replacesNote,
      render: replace,
    },
  ];
  return parts.map(
    (part) =>
      part.rows.length > 0 && (
        <CompareSection
          key={part.title}
          title={part.title}
          note={part.note}
          count={part.rows.length}
          locked={locked}
          {...section(part.rows)}
        >
          {part.rows.map((row) => (
            <Fragment key={keyOf(row)}>{part.render(row)}</Fragment>
          ))}
        </CompareSection>
      ),
  );
}

/** A row label: a checkbox for an item the owner chooses, plain text otherwise. */
export function RowLabel({
  label,
  checked,
  locked,
  onChange,
}: {
  label: string;
  /** Undefined draws the label without a checkbox. */
  checked?: boolean;
  locked: boolean;
  onChange?: (on: boolean) => void;
}) {
  const id = useId();
  if (checked === undefined)
    return <span className="text-muted-foreground">{label}</span>;
  return (
    <div className="flex items-center gap-2 self-start">
      <Checkbox
        id={id}
        checked={checked}
        disabled={locked}
        onCheckedChange={(next) => onChange?.(next === true)}
      />
      <Label htmlFor={id} className="font-normal">
        {label}
      </Label>
    </div>
  );
}

/** Stacked under a checkbox, a value lines up with the label's text. */
function indent(narrow: boolean): string {
  return cn("pl-6", !narrow && "sm:pl-0");
}

/** A label and the one value the apply writes, such as a field the subject has empty. */
export function ValueRow({
  label,
  narrow = false,
  children,
}: {
  label: ReactNode;
  /** Keep the value under the label at every width. */
  narrow?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "grid items-baseline gap-x-4 gap-y-1 py-2",
        !narrow && "sm:grid-cols-compare-fill",
      )}
    >
      {label}
      <div className={cn("min-w-0", indent(narrow))}>{children}</div>
    </div>
  );
}

/** A label, the stored value and the fetched one, read left to right; below sm the pair stays on one line under the label. */
export function PairRow({
  label,
  now,
  fetched,
  indented,
}: {
  label: ReactNode;
  now: ReactNode;
  fetched: ReactNode;
  /** The label carries a checkbox, so the stacked pair lines up with its text. */
  indented: boolean;
}) {
  return (
    <div className="grid items-baseline gap-x-3 gap-y-1 py-2 sm:grid-cols-compare-replace">
      {label}
      <div
        className={cn(
          "flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 sm:contents",
          indented && indent(false),
        )}
      >
        <div className="min-w-0">{now}</div>
        <ArrowRight
          aria-hidden="true"
          className="size-4 shrink-0 self-center text-muted-foreground"
        />
        <div className="min-w-0">{fetched}</div>
      </div>
    </div>
  );
}

/** A fetched value the apply would add, pressed while selected. */
export function ValueToggle({
  label,
  hint,
  pressed,
  disabled,
  leading,
  onPressedChange,
}: {
  label: string;
  hint?: string;
  pressed: boolean;
  disabled: boolean;
  /** Drawn before the label, such as a person's face. */
  leading?: ReactNode;
  onPressedChange: (pressed: boolean) => void;
}) {
  return (
    <Toggle
      variant="outline"
      size="sm"
      pressed={pressed}
      disabled={disabled}
      onPressedChange={onPressedChange}
      title={hint ?? label}
      className="max-w-full font-normal"
    >
      {pressed ? <Check aria-hidden="true" /> : <Plus aria-hidden="true" />}
      {leading}
      <span className="max-w-chip-label truncate">{label}</span>
      {hint !== undefined && <span className="sr-only">{`, ${hint}`}</span>}
    </Toggle>
  );
}

export interface ImageOption {
  value: string;
  label: string;
  image: ReactNode;
}

/** One image among the subject's own and the fetched ones, as a radio group of tiles. */
export function ImageChoice({
  title,
  value,
  options,
  locked,
  wide = false,
  onChange,
}: {
  title: string;
  value: string;
  options: readonly ImageOption[];
  locked: boolean;
  /** Landscape tiles, as for a video's thumbnail. */
  wide?: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <section className="flex flex-col gap-1">
      <h3 className="text-sm font-medium">{title}</h3>
      <RadioGroup
        aria-label={title}
        value={value}
        onValueChange={onChange}
        className="flex flex-wrap gap-2"
      >
        {options.map((option) => (
          <ImageTile key={option.value} option={option} locked={locked} wide={wide} />
        ))}
      </RadioGroup>
    </section>
  );
}

function ImageTile({
  option,
  locked,
  wide,
}: {
  option: ImageOption;
  locked: boolean;
  wide: boolean;
}) {
  const id = useId();
  return (
    <Label
      htmlFor={id}
      className={cn(
        "flex cursor-pointer flex-col items-stretch gap-1 rounded-md p-1 font-normal has-data-[state=checked]:bg-accent",
        wide ? "w-list-thumb-cell" : "w-card-face",
      )}
    >
      {option.image}
      <span className="flex items-center gap-1 text-xs">
        <RadioGroupItem
          id={id}
          value={option.value}
          disabled={locked}
          aria-label={option.label}
        />
        <span className="min-w-0 truncate">{option.label}</span>
      </span>
    </Label>
  );
}

/** The values the apply keeps as they are, folded so the choices stay first. */
export function KeptFold({
  count,
  note,
  children,
}: {
  count: number;
  note: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="flex flex-col gap-1">
      <CollapsibleTrigger asChild>
        <Button
          variant="link"
          size="sm"
          className="h-auto self-start px-0 py-0 text-muted-foreground hover:text-primary"
        >
          {open ? t.common.hideSame : t.common.sameCount(count)}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-1">
        <p className="text-xs text-muted-foreground">{note}</p>
        <div className="flex flex-col divide-y divide-border border-y border-border">
          {children}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
