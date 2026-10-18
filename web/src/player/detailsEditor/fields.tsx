import { Plus, X } from "lucide-react";
import { type KeyboardEvent, useLayoutEffect, useRef } from "react";

import { t, type UiText } from "../../i18n";
import { isComposingKeyEvent } from "../../lib/ime";
import { Button } from "../../ui/shadcn/button";
import { Field, FieldError, FieldTitle } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import { type IdRow, newRowKey } from "./draft";

export interface FieldProblem {
  control: string;
  text: UiText;
}

export function controlId(control: string): string {
  return `details-${control}`;
}

export function errorId(control: string): string {
  return `details-${control}-error`;
}

export function invalidProps(
  problem: FieldProblem | null,
  control: string,
): { "aria-invalid"?: true; "aria-describedby"?: string } {
  if (problem?.control !== control) return {};
  return { "aria-invalid": true, "aria-describedby": errorId(control) };
}

export function Problem({
  problem,
  control,
}: {
  problem: FieldProblem | null;
  control: string;
}) {
  if (problem?.control !== control) return null;
  return <FieldError id={errorId(control)}>{problem.text}</FieldError>;
}

/** Enter saves single-line inputs except during IME composition. */
export function saveOnEnter(save: () => void) {
  return (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter" || isComposingKeyEvent(event)) return;
    event.preventDefault();
    save();
  };
}

export function useFocusAdded() {
  const pending = useRef<string | null>(null);
  useLayoutEffect(() => {
    const control = pending.current;
    if (control === null) return;
    pending.current = null;
    document.getElementById(controlId(control))?.focus();
  });
  return (control: string) => {
    pending.current = control;
  };
}

export interface RowsProps<T> {
  rows: readonly T[];
  onChange: (rows: T[]) => void;
  saving: boolean;
  problem: FieldProblem | null;
  onEnter: ReturnType<typeof saveOnEnter>;
}

export function RemoveRow({
  label,
  saving,
  onRemove,
}: {
  label: UiText;
  saving: boolean;
  onRemove: () => void;
}) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      aria-disabled={saving || undefined}
      onClick={() => {
        if (!saving) onRemove();
      }}
      className="text-muted-foreground"
    >
      <X aria-hidden="true" />
    </Button>
  );
}

export function AddRow({
  label,
  saving,
  onAdd,
  disabled = false,
}: {
  label: UiText;
  saving: boolean;
  onAdd: () => void;
  disabled?: boolean;
}) {
  return (
    <Button
      variant="outline"
      size="sm"
      aria-disabled={saving || undefined}
      disabled={disabled}
      onClick={() => {
        if (!saving) onAdd();
      }}
      className="w-auto self-start"
    >
      <Plus aria-hidden="true" />
      {label}
    </Button>
  );
}

export function IdRows({
  rows,
  onChange,
  saving,
  problem,
  onEnter,
  title = t.player.detailsEditor.ids,
  max,
}: RowsProps<IdRow> & {
  /** Null when the surrounding FieldLegend supplies the label. */
  title?: UiText | null;
  max?: number;
}) {
  const e = t.player.detailsEditor;
  const focusAdded = useFocusAdded();
  const change = (index: number, patch: Partial<IdRow>) =>
    onChange(rows.map((item, at) => (at === index ? { ...item, ...patch } : item)));
  return (
    <Field aria-labelledby={title === null ? undefined : controlId("ids")}>
      {title !== null && <FieldTitle id={controlId("ids")}>{title}</FieldTitle>}
      {rows.map((row, index) => (
        <div key={row.key} className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <Input
              id={controlId(`id-source-${row.key}`)}
              aria-label={e.source}
              placeholder={e.source}
              value={row.source}
              readOnly={saving}
              onChange={(event) => change(index, { source: event.target.value })}
              onKeyDown={onEnter}
              className="h-8 w-1/3 shrink-0"
              {...invalidProps(problem, `id-source-${row.key}`)}
            />
            <Input
              id={controlId(`id-value-${row.key}`)}
              aria-label={e.value}
              placeholder={e.value}
              value={row.value}
              readOnly={saving}
              onChange={(event) => change(index, { value: event.target.value })}
              onKeyDown={onEnter}
              className="h-8 tabular-nums"
              {...invalidProps(problem, `id-value-${row.key}`)}
            />
            <RemoveRow
              label={e.removeId}
              saving={saving}
              onRemove={() => onChange(rows.filter((_, at) => at !== index))}
            />
          </div>
          <Problem problem={problem} control={`id-source-${row.key}`} />
          <Problem problem={problem} control={`id-value-${row.key}`} />
        </div>
      ))}
      <AddRow
        label={e.addId}
        saving={saving}
        disabled={max !== undefined && rows.length >= max}
        onAdd={() => {
          const key = newRowKey("i");
          focusAdded(`id-source-${key}`);
          onChange([...rows, { key, source: "", value: "" }]);
        }}
      />
    </Field>
  );
}
