import { Plus, X } from "lucide-react";
import { type KeyboardEvent, useLayoutEffect, useRef } from "react";

import { t, type UiText } from "../../i18n";
import { isComposingKeyEvent } from "../../lib/ime";
import { Button } from "../../ui/shadcn/button";
import { Field, FieldError, FieldTitle } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import { type IdRow, newRowKey } from "./draft";

// 編集画面の入力が分け合う小さな部品と値。

/** FieldProblem は、保存の誤りの `field` が指した入力とその理由である。 */
export interface FieldProblem {
  /** 入力の要素の id の末尾（`details-${control}`）。 */
  control: string;
  text: UiText;
}

/** controlId は入力の要素の id である。 */
export function controlId(control: string): string {
  return `details-${control}`;
}

/** errorId は入力の理由の要素の id である。 */
export function errorId(control: string): string {
  return `details-${control}-error`;
}

/** invalidProps は、理由がその入力を指すときに入力へ付ける属性である。 */
export function invalidProps(
  problem: FieldProblem | null,
  control: string,
): { "aria-invalid"?: true; "aria-describedby"?: string } {
  if (problem?.control !== control) return {};
  return { "aria-invalid": true, "aria-describedby": errorId(control) };
}

/** Problem は、理由がその入力を指すときだけ、その下に理由を出す。 */
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

/**
 * saveOnEnter は、1 行の入力で Enter を押したら保存する（タイトルのその場編集と同じ。 の Keys）。IME の変換を決める Enter では保存しない。
 */
export function saveOnEnter(save: () => void) {
  return (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter" || isComposingKeyEvent(event)) return;
    event.preventDefault();
    save();
  };
}

/** useFocusAdded は、足した行の最初の入力へフォーカスを移す。 */
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
  /** 上限に届いて足せない。 */
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

/**
 * IdRows は IDs の行（Source と Value と ×）と「Add ID」である。編集画面の `Basics` と、
 * 人物・団体の詳細の窓（`web/src/credits/CreditDetailsDialog`。名前はその窓の `FieldLegend` が
 * 持つので `title` は null、`max` は 20）が使う。
 */
export function IdRows({
  rows,
  onChange,
  saving,
  problem,
  onEnter,
  title = t.player.detailsEditor.ids,
  max,
}: RowsProps<IdRow> & {
  /** 欄の名前。null なら出さない（囲む `FieldSet` の `FieldLegend` が名前を持つとき）。 */
  title?: UiText | null;
  /** 行の上限。届くと「Add ID」を押せない。 */
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
