import { Minus, Plus, X } from "lucide-react";
import { type ReactNode, useState } from "react";

import { t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { foldForMatch } from "../../lib/foldForMatch";
import { Button } from "../../ui/shadcn/button";
import { FieldLegend, FieldSet } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import type { Choice, FieldDraft, PickedValue, PresentValue } from "./draft";

/**
 * A value the selection has, as a chip showing how many videos carry it. Pressing it steps
 * through what Apply does: keep, add to every video, remove from every video.
 */
function PresentChip({
  value,
  total,
  choice,
  disabled,
  onPress,
}: {
  value: PresentValue;
  total: number;
  choice: Choice | undefined;
  disabled: boolean;
  onPress: () => void;
}) {
  const b = t.library.bulkEdit;
  const partial = value.count < total;
  const state =
    choice === "add" ? b.chipAdd : choice === "remove" ? b.chipRemove : b.chipKeep;
  return (
    <li className="max-w-full min-w-0">
      <Button
        variant="outline"
        size="sm"
        data-choice={choice}
        aria-label={b.chipLabel(value.label, value.count, total, state)}
        aria-disabled={disabled || undefined}
        onClick={() => {
          if (!disabled) onPress();
        }}
        className={cn(
          "h-8 max-w-full min-w-0 gap-1.5 px-2 font-normal",
          partial && choice === undefined && "border-dashed",
          choice === "add" &&
            "border-primary bg-primary-soft text-primary hover:bg-primary-soft",
          choice === "remove" &&
            "border-destructive text-destructive line-through hover:bg-card",
        )}
      >
        {choice === "add" && <Plus className="size-3 shrink-0" aria-hidden="true" />}
        {choice === "remove" && <Minus className="size-3 shrink-0" aria-hidden="true" />}
        <span className="min-w-0 truncate">{value.label}</span>
        <span
          aria-hidden="true"
          className={cn(
            "shrink-0 text-xs tabular-nums",
            choice === undefined && "text-muted-foreground",
          )}
        >
          {choice === "add"
            ? b.toAll(total)
            : b.ratio(choice === "remove" ? value.removable : value.count, total)}
        </span>
      </Button>
    </li>
  );
}

function PickedChip({
  value,
  disabled,
  onRemove,
}: {
  value: PickedValue;
  disabled: boolean;
  onRemove: () => void;
}) {
  const b = t.library.bulkEdit;
  return (
    <li className="max-w-full min-w-0">
      <span className="flex h-8 max-w-full min-w-0 items-center gap-1.5 rounded-md border border-primary bg-primary-soft pl-2 text-sm text-primary">
        <Plus className="size-3 shrink-0" aria-hidden="true" />
        <span className="min-w-0 truncate">{value.label}</span>
        {"newName" in value && (
          <span className="shrink-0 text-xs text-muted-foreground">{b.newMark}</span>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={b.unpick(value.label)}
          aria-disabled={disabled || undefined}
          onClick={() => {
            if (!disabled) onRemove();
          }}
          className="size-6 rounded-l-none [&_svg]:size-3"
        >
          <X aria-hidden="true" />
        </Button>
      </span>
    </li>
  );
}

/** A tag list or one credit role: the present values, the picked ones, then the picker. */
export default function ValueField({
  legend,
  present,
  total,
  field,
  disabled,
  onCycle,
  onUnpick,
  picker,
}: {
  legend: UiText;
  present: readonly PresentValue[];
  total: number;
  field: FieldDraft;
  disabled: boolean;
  onCycle: (value: PresentValue) => void;
  onUnpick: (key: string) => void;
  picker: ReactNode;
}) {
  const b = t.library.bulkEdit;
  const [query, setQuery] = useState("");
  const [all, setAll] = useState(false);
  const shown = shownValues(present, field, query, all);
  const hidden = present.length - shown.length;
  return (
    <FieldSet className="gap-2">
      <FieldLegend variant="label">{legend}</FieldLegend>
      {present.length > valuesShown && (
        <Input
          type="search"
          value={query}
          aria-label={b.filter(legend)}
          placeholder={b.filter(legend)}
          className="h-8"
          onChange={(event) => setQuery(event.target.value)}
        />
      )}
      <ul aria-label={legend} className="flex min-w-0 flex-wrap items-center gap-1.5">
        {shown.map((value) => (
          <PresentChip
            key={value.key}
            value={value}
            total={total}
            choice={field.choices.get(value.key)}
            disabled={disabled}
            onPress={() => onCycle(value)}
          />
        ))}
        {field.picked.map((value) => (
          <PickedChip
            key={value.key}
            value={value}
            disabled={disabled}
            onRemove={() => onUnpick(value.key)}
          />
        ))}
        {(hidden > 0 || all) && query.trim() === "" && (
          <li>
            <Button
              variant="ghost"
              size="sm"
              aria-expanded={all}
              className="h-8 px-2"
              onClick={() => setAll((current) => !current)}
            >
              {all ? b.showFewer : b.showAll(present.length)}
            </Button>
          </li>
        )}
        <li className="max-w-full min-w-0">{picker}</li>
      </ul>
    </FieldSet>
  );
}

/** Chips drawn before `Show all`; a field with more also gets a filter. */
const valuesShown = 24;

/**
 * Before `Show all` only the values most videos carry are drawn, in their natural order so a chip
 * does not move when the counts change; a filter keeps the matching ones. A value with a choice
 * always stays so its change remains visible.
 */
function shownValues(
  present: readonly PresentValue[],
  field: FieldDraft,
  query: string,
  all: boolean,
): PresentValue[] {
  const folded = foldForMatch(query.trim());
  if (folded !== "")
    return present.filter(
      (value) =>
        field.choices.has(value.key) || foldForMatch(value.label).includes(folded),
    );
  if (all || present.length <= valuesShown) return [...present];
  const common = new Set(
    [...present]
      .sort((a, b) => b.count - a.count)
      .slice(0, valuesShown)
      .map((value) => value.key),
  );
  return present.filter((value) => common.has(value.key) || field.choices.has(value.key));
}
