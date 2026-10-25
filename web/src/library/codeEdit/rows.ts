import type { components } from "../../api/gen/openapi";

export type CodeEditRow = components["schemas"]["VideoCodeEditRow"];
export type CodeEditItem = components["schemas"]["VideoCodeEditItem"];

/** Where a row's value comes from; only `suggested` rows follow the pattern and upper case. */
export type RowState = "existing" | "suggested" | "blank" | "edited";

/** The code rule's limit in code points, as `WorkInfo.code` on the server. */
export const codeLimit = 100;

/** A pattern that compiled, with the replacement it was typed with. */
export interface CompiledPattern {
  /** Null while the pattern field is empty: suggestions are the file names. */
  regex: RegExp | null;
  replacement: string;
}

export const noPattern: CompiledPattern = { regex: null, replacement: "" };

export interface PatternState {
  compiled: CompiledPattern;
  /** The `RegExp` constructor's message while the typed pattern is invalid. */
  error?: string;
}

/**
 * Compiles the typed pattern; an invalid one keeps `last`, so the suggestions
 * stay at their last valid values while the owner is still typing.
 */
export function nextPattern(
  pattern: string,
  replacement: string,
  last: CompiledPattern,
): PatternState {
  if (pattern === "") return { compiled: { regex: null, replacement } };
  try {
    return { compiled: { regex: new RegExp(pattern, "u"), replacement } };
  } catch (error) {
    return {
      compiled: last,
      error: error instanceof Error ? error.message : String(error),
    };
  }
}

/** An empty replacement takes group 1, else the whole match; a non-matching name stays as it is. */
function suggest(fileTitle: string, { regex, replacement }: CompiledPattern): string {
  if (regex === null) return fileTitle;
  if (replacement !== "") return fileTitle.replace(regex, replacement);
  const match = regex.exec(fileTitle);
  if (match === null) return fileTitle;
  return match[1] ?? match[0];
}

export interface DerivedRow {
  row: CodeEditRow;
  state: RowState;
  /** The value shown and sent: trimmed and cut to `codeLimit` code points. */
  value: string;
  /** Apply sends this row. */
  writes: boolean;
  /** The value was longer than `codeLimit` code points. */
  cut: boolean;
  /** Another row has the same non-empty value; it stays writable. */
  duplicate: boolean;
}

export interface DeriveInput {
  rows: readonly CodeEditRow[];
  pattern: CompiledPattern;
  upperCase: boolean;
  /** What the owner typed, by video ID; deleting an entry restores the suggestion or the code. */
  typed: ReadonlyMap<number, string>;
}

function initialState(row: CodeEditRow): RowState {
  if (row.code !== undefined && row.code !== "") return "existing";
  return row.video.fileTitle === undefined ? "blank" : "suggested";
}

function rawValue(row: CodeEditRow, state: RowState, input: DeriveInput): string {
  switch (state) {
    case "edited":
      return input.typed.get(row.video.id) ?? "";
    case "existing":
      return row.code ?? "";
    case "blank":
      return "";
    case "suggested": {
      const value = suggest(row.video.fileTitle ?? "", input.pattern);
      // Locale-independent, so a product code does not change with the browser locale.
      return input.upperCase ? value.toUpperCase() : value;
    }
  }
}

function writes(row: CodeEditRow, state: RowState, value: string): boolean {
  if (value === "") return false;
  if (state === "suggested") return true;
  return state === "edited" && value !== (row.code ?? "");
}

function deriveOne(row: CodeEditRow, input: DeriveInput): Omit<DerivedRow, "duplicate"> {
  const state = input.typed.has(row.video.id) ? "edited" : initialState(row);
  const trimmed = rawValue(row, state, input).trim();
  const points = Array.from(trimmed);
  const cut = points.length > codeLimit;
  const value = cut ? points.slice(0, codeLimit).join("") : trimmed;
  return { row, state, value, writes: writes(row, state, value), cut };
}

/** Each row's state, value and marks for the current pattern, option and typed values. */
export function deriveRows(input: DeriveInput): DerivedRow[] {
  const derived = input.rows.map((row) => deriveOne(row, input));
  const counts = new Map<string, number>();
  for (const { value } of derived) {
    if (value !== "") counts.set(value, (counts.get(value) ?? 0) + 1);
  }
  return derived.map((row) => ({ ...row, duplicate: (counts.get(row.value) ?? 0) > 1 }));
}

/** The items Apply sends; empty rows are left out, so this editor never clears a code. */
export function applyItems(rows: readonly DerivedRow[]): CodeEditItem[] {
  return rows
    .filter((row) => row.writes)
    .map(({ row, value }) => ({
      videoId: row.video.id,
      contentVersion: row.video.contentVersion ?? "",
      code: value,
    }));
}
