import { closestCenter, DndContext, type DragEndEvent } from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { ArrowUpToLine, X } from "lucide-react";
import type { ReactNode, Ref } from "react";

import type { CreditName } from "../api/credits";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { newRowKey } from "../player/detailsEditor/draft";
import {
  AddRow,
  controlId,
  type FieldProblem,
  invalidProps,
  Problem,
  type RowsProps,
  useFocusAdded,
} from "../player/detailsEditor/fields";
import {
  ReorderHandle,
  reorderAnnouncements,
  useReorderSensors,
  useSortableRow,
  verticalOnly,
} from "../videoList/reorder";
import { Button } from "../ui/shadcn/button";
import { Field, FieldDescription, FieldTitle } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

const text = t.credits.names;

export const maxNames = 50;

/** One editable name; the row at index 0 is the display name. */
export interface NameRow {
  key: string;
  name: string;
  language: string;
}

/** The control of the list's own error, under `Add name`. */
export const namesControl = "names";

export function nameControl(part: "name" | "language", key: string): string {
  return `name-${part}-${key}`;
}

function blankRow(): NameRow {
  return { key: newRowKey("n"), name: "", language: "" };
}

function isBlank(row: NameRow): boolean {
  return row.name.trim() === "" && row.language.trim() === "";
}

/** The rows of a loaded list; an empty list shows one blank row in the display name's slot. */
export function nameRowsOf(names: readonly CreditName[]): NameRow[] {
  if (names.length === 0) return [blankRow()];
  return names.map((entry) => ({
    key: newRowKey("n"),
    name: entry.name,
    language: entry.language ?? "",
  }));
}

/**
 * The list a save sends, trimmed and with each sent row's key. A lone blank row is the empty list;
 * otherwise every row is sent, so a blank one gets the server's reason at its own index.
 */
export function nameRowsToSend(rows: readonly NameRow[]): {
  values: CreditName[];
  keys: string[];
} {
  if (rows.length === 1 && isBlank(rows[0]!)) return { values: [], keys: [] };
  return {
    values: rows.map((row) => {
      const language = row.language.trim();
      return language === ""
        ? { name: row.name.trim() }
        : { name: row.name.trim(), language };
    }),
    keys: rows.map((row) => row.key),
  };
}

/** The control a `names` path names, read through the keys of the rows that were sent. */
export function nameRowsControl(
  field: string,
  keys: readonly string[],
): string | undefined {
  if (field === namesControl) return namesControl;
  const entry = /^names\[(\d+)\]\.(name|language)$/.exec(field);
  if (entry === null) return undefined;
  const key = keys[Number(entry[1])];
  return key === undefined
    ? undefined
    : nameControl(entry[2] as "name" | "language", key);
}

function titleOf(row: NameRow): string {
  const name = row.name.trim();
  return name === "" ? text.blank : name;
}

/**
 * The names of a person or an organization as one sortable list: the row under `Display name`
 * is the display name, and the slot labels stay put while rows slide between them.
 */
export function NameRows({
  rows,
  onChange,
  saving,
  problem,
  onEnter,
  title,
  firstPlaceholder,
  firstRef,
}: RowsProps<NameRow> & {
  /** Omitted when the surrounding FieldLegend supplies the label. */
  title?: UiText;
  /** Shown in the first row while it is blank, as a provisional person's number. */
  firstPlaceholder?: string;
  firstRef?: Ref<HTMLInputElement>;
}) {
  const sensors = useReorderSensors();
  const focusAdded = useFocusAdded();
  const items = rows.map((row) => ({ id: row.key, row }));
  const change = (key: string, patch: Partial<NameRow>) =>
    onChange(rows.map((row) => (row.key === key ? { ...row, ...patch } : row)));
  const remove = (key: string) => {
    const kept = rows.filter((row) => row.key !== key);
    onChange(kept.length === 0 ? [blankRow()] : kept);
  };
  const promote = (key: string) => {
    const row = rows.find((each) => each.key === key);
    if (row === undefined) return;
    focusAdded(nameControl("name", key));
    onChange([row, ...rows.filter((each) => each.key !== key)]);
  };
  const dragEnd = ({ active, over }: DragEndEvent) => {
    if (over === null || active.id === over.id) return;
    const from = rows.findIndex((row) => row.key === active.id);
    const to = rows.findIndex((row) => row.key === over.id);
    if (from < 0 || to < 0) return;
    onChange(arrayMove([...rows], from, to));
  };
  const rowOf = (row: NameRow, index: number) => (
    <NameRowItem
      key={row.key}
      row={row}
      first={index === 0}
      saving={saving}
      problem={problem}
      onEnter={onEnter}
      placeholder={index === 0 ? firstPlaceholder : undefined}
      nameRef={index === 0 ? firstRef : undefined}
      onChange={(patch) => change(row.key, patch)}
      onPromote={() => promote(row.key)}
      onRemove={() => remove(row.key)}
    />
  );
  return (
    <Field aria-labelledby={title === undefined ? undefined : controlId(namesControl)}>
      {title !== undefined && (
        <FieldTitle id={controlId(namesControl)}>{title}</FieldTitle>
      )}
      <FieldDescription>{text.description}</FieldDescription>
      <div className="flex flex-col gap-4">
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          modifiers={[verticalOnly]}
          onDragEnd={dragEnd}
          accessibility={{
            announcements: reorderAnnouncements(items, (item) => titleOf(item.row), text),
            screenReaderInstructions: { draggable: text.reorderInstructions },
          }}
        >
          <SortableContext
            items={rows.map((row) => row.key)}
            strategy={verticalListSortingStrategy}
          >
            <Slot id="names-first" label={text.displayName}>
              {rows.slice(0, 1).map(rowOf)}
            </Slot>
            <Slot id="names-others" label={text.otherNames}>
              {rows.slice(1).map((row, index) => rowOf(row, index + 1))}
            </Slot>
          </SortableContext>
        </DndContext>
        <AddRow
          label={text.add}
          saving={saving}
          disabled={rows.length >= maxNames}
          onAdd={() => {
            const added = blankRow();
            focusAdded(nameControl("name", added.key));
            onChange([...rows, added]);
          }}
        />
      </div>
      <Problem problem={problem} control={namesControl} />
    </Field>
  );
}

/** A fixed label over its rows; it is not sortable, so rows slide past it into the next slot. */
function Slot({
  id,
  label,
  children,
}: {
  id: string;
  label: UiText;
  children: ReactNode;
}) {
  return (
    <div role="group" aria-labelledby={controlId(id)} className="flex flex-col gap-2">
      <FieldTitle id={controlId(id)}>{label}</FieldTitle>
      {children}
    </div>
  );
}

function NameRowItem({
  row,
  first,
  saving,
  problem,
  onEnter,
  placeholder,
  nameRef,
  onChange,
  onPromote,
  onRemove,
}: {
  row: NameRow;
  first: boolean;
  saving: boolean;
  problem: FieldProblem | null;
  onEnter: RowsProps<NameRow>["onEnter"];
  placeholder?: string;
  nameRef?: Ref<HTMLInputElement>;
  onChange: (patch: Partial<NameRow>) => void;
  onPromote: () => void;
  onRemove: () => void;
}) {
  const { rowRef, style, isDragging, handle } = useSortableRow(row.key, saving);
  const name = row.name.trim();
  const nameId = nameControl("name", row.key);
  const languageId = nameControl("language", row.key);
  return (
    <div
      ref={rowRef}
      style={style}
      data-name-row=""
      className={cn(
        "flex flex-col gap-1 rounded-md",
        isDragging && "relative z-10 bg-popover shadow-elevated",
      )}
    >
      <div className="flex items-start gap-2">
        <ReorderHandle
          handle={handle}
          label={text.reorder(titleOf(row))}
          disabled={saving}
        />
        <div className="grid min-w-0 flex-1 gap-2 sm:grid-cols-3">
          <Input
            ref={nameRef}
            id={controlId(nameId)}
            aria-label={text.name}
            placeholder={placeholder}
            value={row.name}
            readOnly={saving}
            onChange={(event) => onChange({ name: event.target.value })}
            onKeyDown={onEnter}
            className="h-8 sm:col-span-2"
            {...invalidProps(problem, nameId)}
          />
          <Input
            id={controlId(languageId)}
            aria-label={text.language}
            placeholder={text.languagePlaceholder}
            value={row.language}
            readOnly={saving}
            onChange={(event) => onChange({ language: event.target.value })}
            onKeyDown={onEnter}
            className="h-8"
            {...invalidProps(problem, languageId)}
          />
        </div>
        {first ? (
          // Keeps the inputs of every row ending on the same line.
          <span aria-hidden="true" className="size-8 shrink-0" />
        ) : (
          <RowButton
            label={name === "" ? text.promoteBlank : text.promote(name)}
            saving={saving}
            onPress={onPromote}
          >
            <ArrowUpToLine aria-hidden="true" />
          </RowButton>
        )}
        <RowButton
          label={name === "" ? text.removeBlank : text.remove(name)}
          saving={saving}
          onPress={onRemove}
        >
          <X aria-hidden="true" />
        </RowButton>
      </div>
      <Problem problem={problem} control={nameId} />
      <Problem problem={problem} control={languageId} />
    </div>
  );
}

function RowButton({
  label,
  saving,
  onPress,
  children,
}: {
  label: UiText;
  saving: boolean;
  onPress: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="shrink-0 text-muted-foreground"
          aria-label={label}
          aria-disabled={saving || undefined}
          onClick={() => {
            if (!saving) onPress();
          }}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
