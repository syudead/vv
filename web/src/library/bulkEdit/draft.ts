import type { WorkDateKind } from "../../api/workInfo";
import type {
  VideoCreditEdit,
  VideoDateEdit,
  VideoEditChanges,
  VideoEditSummary,
} from "../../api/videoEdits";
import type { TagRef } from "../../api/tags";

/** What Apply does with one value; a value without a choice stays as each video has it. */
export type Choice = "add" | "remove";

export type CreditKind = VideoCreditEdit["kind"];

/** `tags`, or a credit role such as `person:performer`. */
export type FieldId = "tags" | `${CreditKind}:${string}`;

/** A value the selection already has, with how many of the selected videos carry it. */
export interface PresentValue {
  key: string;
  label: string;
  count: number;
  /** Videos whose copy Remove can drop; folder-derived tags cannot be removed. */
  removable: number;
}

/** A value picked in the editor that no selected video has yet; it is always added. */
export type PickedValue =
  | { key: string; label: string; id: number }
  | { key: string; label: string; newName: string };

export interface FieldDraft {
  choices: ReadonlyMap<string, Choice>;
  picked: readonly PickedValue[];
}

export type DateDraft = { mode: "set"; value: string } | { mode: "clear" };

export interface BulkDraft {
  fields: ReadonlyMap<FieldId, FieldDraft>;
  dates: Partial<Record<WorkDateKind, DateDraft>>;
  /** True makes every target public, false private; undefined keeps each video's visibility. */
  public?: boolean;
}

export const emptyDraft: BulkDraft = { fields: new Map(), dates: {} };

const emptyField: FieldDraft = { choices: new Map(), picked: [] };

export function fieldOf(draft: BulkDraft, id: FieldId): FieldDraft {
  return draft.fields.get(id) ?? emptyField;
}

function withField(draft: BulkDraft, id: FieldId, field: FieldDraft): BulkDraft {
  const fields = new Map(draft.fields);
  fields.set(id, field);
  return { ...draft, fields };
}

/** Add to every video is offered only while some lack the value, Remove only while some can lose it. */
export function nextChoice(
  value: PresentValue,
  total: number,
  current: Choice | undefined,
): Choice | undefined {
  const order: Choice[] = [];
  if (value.count < total) order.push("add");
  if (value.removable > 0) order.push("remove");
  const index = current === undefined ? -1 : order.indexOf(current);
  return order[index + 1];
}

export function cycleValue(
  draft: BulkDraft,
  id: FieldId,
  value: PresentValue,
  total: number,
): BulkDraft {
  const field = fieldOf(draft, id);
  const choices = new Map(field.choices);
  const next = nextChoice(value, total, choices.get(value.key));
  if (next === undefined) choices.delete(value.key);
  else choices.set(value.key, next);
  return withField(draft, id, { ...field, choices });
}

/** Picking a value the selection already has marks it for every video instead of listing it twice. */
export function pickValue(
  draft: BulkDraft,
  id: FieldId,
  value: PickedValue,
  present: readonly PresentValue[],
): BulkDraft {
  const field = fieldOf(draft, id);
  if (present.some((item) => item.key === value.key)) {
    const choices = new Map(field.choices);
    choices.set(value.key, "add");
    return withField(draft, id, { ...field, choices });
  }
  if (field.picked.some((item) => item.key === value.key)) return draft;
  return withField(draft, id, { ...field, picked: [...field.picked, value] });
}

export function unpickValue(draft: BulkDraft, id: FieldId, key: string): BulkDraft {
  const field = fieldOf(draft, id);
  return withField(draft, id, {
    ...field,
    picked: field.picked.filter((item) => item.key !== key),
  });
}

export function setDate(
  draft: BulkDraft,
  kind: WorkDateKind,
  date: DateDraft | undefined,
): BulkDraft {
  const dates = { ...draft.dates };
  if (date === undefined) delete dates[kind];
  else dates[kind] = date;
  return { ...draft, dates };
}

export function setPublic(draft: BulkDraft, value: boolean | undefined): BulkDraft {
  const next = { ...draft };
  if (value === undefined) delete next.public;
  else next.public = value;
  return next;
}

export function tagKey(id: number): string {
  return `t${String(id)}`;
}

export function creditKey(id: number): string {
  return `c${String(id)}`;
}

export function idOfKey(key: string): number {
  return Number(key.slice(1));
}

export function presentTags(summary: VideoEditSummary): PresentValue[] {
  return summary.tags.map((item) => ({
    key: tagKey(item.tag.id),
    label: item.tag.name,
    count: item.count,
    removable: item.manualCount,
  }));
}

/** Present credits of one role; a provisional person's empty name is labelled by the caller. */
export function presentCredits(
  summary: VideoEditSummary,
  kind: CreditKind,
  role: string,
  unnamed: string,
): PresentValue[] {
  const items =
    kind === "person"
      ? summary.people
          .filter((item) => item.role === role)
          .map((item) => ({ ref: item.person, count: item.count }))
      : summary.organizations
          .filter((item) => item.role === role)
          .map((item) => ({ ref: item.organization, count: item.count }));
  return items.map(({ ref, count }) => ({
    key: creditKey(ref.id),
    label: ref.provisional ? unnamed : ref.name,
    count,
    removable: count,
  }));
}

/** One line of the change list under the editor, in field order. */
export interface ChangeLine {
  field: FieldId | WorkDateKind | "public";
  label: string;
  action: Choice | "set" | "clear" | "public" | "private";
  /** Videos the line changes, as far as the summary tells. */
  videos: number;
}

const dateKinds: readonly WorkDateKind[] = ["release", "recorded", "published"];

function fieldLines(
  id: FieldId,
  field: FieldDraft,
  present: readonly PresentValue[],
  total: number,
): ChangeLine[] {
  const lines: ChangeLine[] = [];
  for (const value of present) {
    const choice = field.choices.get(value.key);
    if (choice === "add" && value.count < total)
      lines.push({
        field: id,
        label: value.label,
        action: "add",
        videos: total - value.count,
      });
    if (choice === "remove" && value.removable > 0)
      lines.push({
        field: id,
        label: value.label,
        action: "remove",
        videos: value.removable,
      });
  }
  for (const value of field.picked)
    lines.push({ field: id, label: value.label, action: "add", videos: total });
  return lines;
}

/**
 * The effective changes, dropping choices that no longer change anything after the targets
 * changed. `present` gives each field's present values in display order.
 */
export function changeLines(
  draft: BulkDraft,
  summary: VideoEditSummary,
  present: ReadonlyMap<FieldId, readonly PresentValue[]>,
): ChangeLine[] {
  const lines: ChangeLine[] = [];
  for (const [id, values] of present) {
    lines.push(...fieldLines(id, fieldOf(draft, id), values, summary.total));
  }
  for (const kind of dateKinds) {
    const date = draft.dates[kind];
    if (date === undefined) continue;
    const current = summary.dates.find((item) => item.kind === kind);
    if (date.mode === "clear") {
      if ((current?.count ?? 0) > 0)
        lines.push({
          field: kind,
          label: "",
          action: "clear",
          videos: current?.count ?? 0,
        });
      continue;
    }
    const value = date.value.trim();
    if (value === "") continue;
    const same = current?.date === value ? current.count : 0;
    if (same < summary.total)
      lines.push({
        field: kind,
        label: value,
        action: "set",
        videos: summary.total - same,
      });
  }
  if (draft.public !== undefined) {
    const changed = draft.public ? summary.total - summary.public : summary.public;
    if (changed > 0)
      lines.push({
        field: "public",
        label: "",
        action: draft.public ? "public" : "private",
        videos: changed,
      });
  }
  return lines;
}

function creditEdit(
  id: FieldId,
  field: FieldDraft,
  present: readonly PresentValue[],
  created: ReadonlyMap<string, number>,
): VideoCreditEdit | undefined {
  const [kind, role] = id.split(":") as [CreditKind, string];
  const add: number[] = [];
  const remove: number[] = [];
  for (const value of present) {
    const choice = field.choices.get(value.key);
    if (choice === "add") add.push(idOfKey(value.key));
    if (choice === "remove") remove.push(idOfKey(value.key));
  }
  for (const value of field.picked) {
    const picked = "id" in value ? value.id : created.get(value.key);
    if (picked !== undefined) add.push(picked);
  }
  if (add.length + remove.length === 0) return undefined;
  return {
    kind,
    role,
    ...(add.length > 0 && { add }),
    ...(remove.length > 0 && { remove }),
  };
}

/**
 * Build the request from the draft. `created` maps a new credit's picked key to the ID created
 * for it before sending; `tagRefs` resolves removed tags for the list overlay.
 */
export function buildChanges(
  draft: BulkDraft,
  summary: VideoEditSummary,
  present: ReadonlyMap<FieldId, readonly PresentValue[]>,
  created: ReadonlyMap<string, number>,
): { changes: VideoEditChanges; removedTags: TagRef[] } {
  const changes: VideoEditChanges = {};
  const removedTags: TagRef[] = [];
  const credits: VideoCreditEdit[] = [];
  for (const [id, values] of present) {
    const field = fieldOf(draft, id);
    if (id !== "tags") {
      const edit = creditEdit(id, field, values, created);
      if (edit !== undefined) credits.push(edit);
      continue;
    }
    const add: NonNullable<VideoEditChanges["tags"]>["add"] = [];
    const remove: number[] = [];
    for (const value of values) {
      const choice = field.choices.get(value.key);
      if (choice === "add") add.push({ id: idOfKey(value.key) });
      if (choice === "remove") {
        remove.push(idOfKey(value.key));
        const tag = summary.tags.find((item) => tagKey(item.tag.id) === value.key)?.tag;
        if (tag !== undefined) removedTags.push(tag);
      }
    }
    for (const value of field.picked)
      add.push("id" in value ? { id: value.id } : { name: value.newName });
    if (add.length + remove.length > 0)
      changes.tags = {
        ...(add.length > 0 && { add }),
        ...(remove.length > 0 && { remove }),
      };
  }
  if (credits.length > 0) changes.credits = credits;
  const dates = dateKinds.flatMap((kind): VideoDateEdit[] => {
    const date = draft.dates[kind];
    if (date === undefined) return [];
    if (date.mode === "clear") return [{ kind, date: null }];
    const value = date.value.trim();
    return value === "" ? [] : [{ kind, date: value }];
  });
  if (dates.length > 0) changes.dates = dates;
  if (draft.public !== undefined) changes.public = draft.public;
  return { changes, removedTags };
}

/** New credit names that must be created before Apply, once per kind. */
export function newCredits(
  draft: BulkDraft,
): { key: string; kind: CreditKind; name: string }[] {
  const seen = new Set<string>();
  const out: { key: string; kind: CreditKind; name: string }[] = [];
  for (const [id, field] of draft.fields) {
    if (id === "tags") continue;
    const kind = id.split(":")[0] as CreditKind;
    for (const value of field.picked) {
      if (!("newName" in value) || seen.has(value.key)) continue;
      seen.add(value.key);
      out.push({ key: value.key, kind, name: value.newName });
    }
  }
  return out;
}
