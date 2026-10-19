import type { PersonPluginItem } from "../../api/personPluginFetches";
import type {
  PersonLookupSelection,
  PersonPluginResult,
  SkippedWorkInfoValue,
  WorkInfoFetch,
  WorkInfoFetchedPerson,
  WorkInfoField,
} from "../../api/workInfoPlugins";
import {
  compareRows,
  initialItems,
  type PersonCompareCurrent,
} from "../../credits/person/pluginFetch/personCompareRows";

/** A planned lookup's progress; a shown result carries the items the owner keeps checked. */
export type LookupState =
  | { status: "looking" }
  | {
      status: "shown";
      result: PersonPluginResult;
      /** The result has an item the apply would write. */
      changes: boolean;
      items: PersonPluginItem[];
    }
  | { status: "failed"; error: unknown }
  | { status: "changed" };

type Entry = WorkInfoFetchedPerson & { at: number };

function roleField(role: WorkInfoFetchedPerson["role"]): WorkInfoField {
  return `people.${role}`;
}

/** The name a chip shows: the matched person's current name, or the fetched one. */
export function entryName(entry: WorkInfoFetchedPerson): string {
  return entry.match.name ?? entry.name;
}

function entriesOf(result: WorkInfoFetch, index: number): Entry[] {
  return result.people
    .map((entry, at) => ({ ...entry, at }))
    .filter((entry) => entry.lookup === index);
}

/** The role rows that link a lookup's person; its items apply only through one of them. */
export function lookupFields(result: WorkInfoFetch, index: number): WorkInfoField[] {
  return [...new Set(entriesOf(result, index).map((entry) => roleField(entry.role)))];
}

export function lookupEnabled(
  result: WorkInfoFetch,
  index: number,
  selection: readonly WorkInfoField[],
): boolean {
  return lookupFields(result, index).some((field) => selection.includes(field));
}

/** A lookup of a person the apply creates compares with nothing. */
export function lookupCurrent(result: PersonPluginResult): PersonCompareCurrent {
  return result.current ?? { aliases: [], externalIds: [], profile: {} };
}

export function shownLookup(result: PersonPluginResult): LookupState {
  const items = initialItems(compareRows(lookupCurrent(result), result.person));
  return { status: "shown", result, changes: items.length > 0, items };
}

function hasChanges(state: LookupState): boolean {
  return state.status === "shown" && state.changes;
}

/** Role rows that a shown lookup with something to apply makes selectable. */
export function lookupChangeFields(
  result: WorkInfoFetch,
  states: readonly LookupState[],
): Set<WorkInfoField> {
  const fields = new Set<WorkInfoField>();
  states.forEach((state, index) => {
    if (hasChanges(state))
      for (const field of lookupFields(result, index)) fields.add(field);
  });
  return fields;
}

/** The apply's personLookups: checked items of shown lookups that a checked role row links. */
export function lookupSelections(
  result: WorkInfoFetch,
  states: readonly LookupState[],
  selection: readonly WorkInfoField[],
): PersonLookupSelection[] {
  return states.flatMap((state, index) =>
    state.status === "shown" &&
    state.items.length > 0 &&
    lookupEnabled(result, index, selection)
      ? [{ index, items: [...state.items] }]
      : [],
  );
}

/**
 * Lookups an apply now would leave out because they are still running: one a checked role row
 * links, or one whose role row cannot be chosen yet, since its result would select that row.
 */
export function cutShort(
  result: WorkInfoFetch,
  states: readonly LookupState[],
  selection: readonly WorkInfoField[],
  selectable: (field: WorkInfoField) => boolean,
): number {
  return states.filter(
    (state, index) =>
      state.status === "looking" &&
      lookupFields(result, index).some(
        (field) => selection.includes(field) || !selectable(field),
      ),
  ).length;
}

/** A role row whose chips carry a lookup block or note, which Same as now would hide. */
export function hasRoleLookups(result: WorkInfoFetch, field: WorkInfoField): boolean {
  return result.people.some(
    (entry) =>
      roleField(entry.role) === field &&
      (entry.lookup !== undefined || entry.lookupLimited === true),
  );
}

export interface LookupBlockPlace {
  index: number;
  names: string[];
}

export interface LookupNote {
  key: string;
  name: string;
  /** The role row whose block shows the shared lookup; absent for an entry over the limit. */
  sharedUnder?: WorkInfoField;
}

export interface RoleLookups {
  blocks: LookupBlockPlace[];
  notes: LookupNote[];
  /** Chip keys of entries with a lookup, whose fill line fills only what the lookup leaves empty. */
  lookedUp: Set<string>;
}

/**
 * What a role row draws under its chips: a block for each lookup whose first entry is in the row,
 * a note for an entry whose shared lookup sits in an earlier row, and one for an entry over the limit.
 * Chip keys are positions within the role, as the comparison's rows number them.
 */
export function roleLookups(result: WorkInfoFetch, field: WorkInfoField): RoleLookups {
  const row = result.people
    .map((entry, at) => ({ ...entry, at }))
    .filter((entry) => roleField(entry.role) === field);
  const blocks: LookupBlockPlace[] = [];
  const notes: LookupNote[] = [];
  const lookedUp = new Set<string>();
  row.forEach((entry, position) => {
    const key = String(position);
    if (entry.lookupLimited === true) notes.push({ key, name: entryName(entry) });
    if (entry.lookup === undefined) return;
    lookedUp.add(key);
    const first = entriesOf(result, entry.lookup)[0];
    if (first !== undefined && roleField(first.role) !== field) {
      notes.push({ key, name: entryName(entry), sharedUnder: roleField(first.role) });
      return;
    }
    const block = blocks.find((item) => item.index === entry.lookup);
    if (block === undefined)
      blocks.push({ index: entry.lookup, names: [entryName(entry)] });
    else if (!block.names.includes(entryName(entry))) block.names.push(entryName(entry));
  });
  return { blocks, notes, lookedUp };
}

/** The person a skipped lookup value belongs to, named as its role row's chip. */
export function skippedLookupName(
  result: WorkInfoFetch,
  value: SkippedWorkInfoValue,
): string | undefined {
  if (value.lookup === undefined) return undefined;
  const entry = entriesOf(result, value.lookup).find(
    (item) => roleField(item.role) === value.field,
  );
  return entry === undefined ? undefined : entryName(entry);
}
