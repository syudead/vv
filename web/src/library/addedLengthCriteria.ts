import { useMemo } from "react";

import type { AddedLengthFilterParams } from "../api/client";
import { formatNumber, t, type UiText } from "../i18n";

/** Library-only filters stay out of shared ListCriteria and folder URLs; guests may use them. */

export const ADDED_WITHIN_PARAM = "addedWithin";
export const MIN_DURATION_PARAM = "minDuration";
export const MAX_DURATION_PARAM = "maxDuration";

export const addedLengthParams: readonly string[] = [
  ADDED_WITHIN_PARAM,
  MIN_DURATION_PARAM,
  MAX_DURATION_PARAM,
];

export const MIN_ADDED_WITHIN = 1;
export const MAX_ADDED_WITHIN = 36500;
export const MAX_DURATION_SECONDS = 360000;

export type AddedLengthFilter = AddedLengthFilterParams;

/** Inclusive duration bounds in seconds. */
export interface LengthRange {
  minDuration?: number;
  maxDuration?: number;
}

const addedPresets: readonly number[] = [1, 7, 30, 90, 365];

type LengthPresetKey = "under5" | "from5to20" | "from20to60" | "overHour";

const lengthPresets: readonly { key: LengthPresetKey; range: LengthRange }[] = [
  { key: "under5", range: { maxDuration: 299 } },
  { key: "from5to20", range: { minDuration: 300, maxDuration: 1199 } },
  { key: "from20to60", range: { minDuration: 1200, maxDuration: 3599 } },
  { key: "overHour", range: { minDuration: 3600 } },
];

/** Read the first URL value as a nonnegative integer; callers enforce range limits. */
function readWhole(raw: readonly string[]): number | undefined | "invalid" {
  const value = raw[0];
  if (value === undefined) return undefined;
  if (!/^\d{1,9}$/.test(value)) return "invalid";
  return Number(value);
}

export function validAddedWithin(days: number): boolean {
  return Number.isInteger(days) && days >= MIN_ADDED_WITHIN && days <= MAX_ADDED_WITHIN;
}

function validDuration(seconds: number): boolean {
  return Number.isInteger(seconds) && seconds >= 0 && seconds <= MAX_DURATION_SECONDS;
}

export function validAddedLength(filter: AddedLengthFilter): boolean {
  const { addedWithin, minDuration, maxDuration } = filter;
  if (addedWithin !== undefined && !validAddedWithin(addedWithin)) return false;
  if (minDuration !== undefined && !validDuration(minDuration)) return false;
  if (maxDuration !== undefined && !validDuration(maxDuration)) return false;
  return !(
    minDuration !== undefined &&
    maxDuration !== undefined &&
    minDuration > maxDuration
  );
}

export interface ParsedAddedLength {
  filter: AddedLengthFilter;
  // Replace invalid URL values without adding browser history entries.
  dropped: boolean;
}

// Drop invalid bounds individually, or both bounds when the range is reversed.
export function parseAddedLength(search: URLSearchParams): ParsedAddedLength {
  let dropped = false;
  const read = (name: string, valid: (value: number) => boolean) => {
    const value = readWhole(search.getAll(name));
    if (value === undefined) return undefined;
    if (value === "invalid" || !valid(value)) {
      dropped = true;
      return undefined;
    }
    return value;
  };
  const addedWithin = read(ADDED_WITHIN_PARAM, validAddedWithin);
  let minDuration = read(MIN_DURATION_PARAM, validDuration);
  let maxDuration = read(MAX_DURATION_PARAM, validDuration);
  if (
    minDuration !== undefined &&
    maxDuration !== undefined &&
    minDuration > maxDuration
  ) {
    dropped = true;
    minDuration = undefined;
    maxDuration = undefined;
  }
  return { filter: { addedWithin, minDuration, maxDuration }, dropped };
}

// Preserve object identity while the parsed values are unchanged.
export function useAddedLength(search: string): ParsedAddedLength {
  const parsed = parseAddedLength(new URLSearchParams(search));
  const { addedWithin, minDuration, maxDuration } = parsed.filter;
  const filter = useMemo(
    () => ({ addedWithin, minDuration, maxDuration }),
    [addedWithin, minDuration, maxDuration],
  );
  return { filter, dropped: parsed.dropped };
}

export function hasAddedLength(filter: AddedLengthFilter): boolean {
  return (
    filter.addedWithin !== undefined ||
    filter.minDuration !== undefined ||
    filter.maxDuration !== undefined
  );
}

export function addedLengthCount(filter: AddedLengthFilter): number {
  const added = filter.addedWithin === undefined ? 0 : 1;
  const length =
    filter.minDuration === undefined && filter.maxDuration === undefined ? 0 : 1;
  return added + length;
}

export function addedLengthSignature(filter: AddedLengthFilter): string {
  return [filter.addedWithin, filter.minDuration, filter.maxDuration]
    .map((value) => String(value ?? ""))
    .join("\u0000");
}

function written(value: number | undefined): string[] {
  return value === undefined ? [] : [String(value)];
}

export function withAddedWithin(days: number | undefined): Record<string, string[]> {
  return { [ADDED_WITHIN_PARAM]: written(days) };
}

export function withLength(range: LengthRange): Record<string, string[]> {
  return {
    [MIN_DURATION_PARAM]: written(range.minDuration),
    [MAX_DURATION_PARAM]: written(range.maxDuration),
  };
}

export function addedLengthValues(filter: AddedLengthFilter): Record<string, string[]> {
  return { ...withAddedWithin(filter.addedWithin), ...withLength(filter) };
}

export const clearedAddedLength: Readonly<Record<string, string[]>> = addedLengthValues(
  {},
);

export interface AddedLengthOption<T> {
  value: string;
  label: UiText;
  choice: T;
}

const ANY_OPTION = "any";
// Represent URL values that do not match a preset.
const CUSTOM_OPTION = "custom";

// Keep a custom URL value selectable until the user chooses another option.
export function addedOptions(current: number | undefined): {
  options: AddedLengthOption<number | undefined>[];
  value: string;
} {
  const words = t.list.filter.addedOptions;
  const options: AddedLengthOption<number | undefined>[] = [
    { value: ANY_OPTION, label: words.any, choice: undefined },
    ...addedPresets.map((days) => ({
      value: String(days),
      label: words.preset(days),
      choice: days,
    })),
  ];
  if (current === undefined) return { options, value: ANY_OPTION };
  if (addedPresets.includes(current)) return { options, value: String(current) };
  options.push({ value: CUSTOM_OPTION, label: words.days(current), choice: current });
  return { options, value: CUSTOM_OPTION };
}

function sameRange(a: LengthRange, b: LengthRange): boolean {
  return a.minDuration === b.minDuration && a.maxDuration === b.maxDuration;
}

function minutes(seconds: number): UiText {
  return formatNumber(Math.round(seconds / 6) / 10);
}

function customLengthLabel(range: LengthRange): UiText {
  const words = t.list.filter.lengthOptions;
  const { minDuration, maxDuration } = range;
  if (minDuration !== undefined && maxDuration !== undefined) {
    return words.between(minutes(minDuration), minutes(maxDuration));
  }
  if (maxDuration !== undefined) return words.under(minutes(maxDuration));
  return words.over(minutes(minDuration ?? 0));
}

// Include custom duration ranges from the URL.
export function lengthOptions(current: LengthRange): {
  options: AddedLengthOption<LengthRange>[];
  value: string;
} {
  const words = t.list.filter.lengthOptions;
  const options: AddedLengthOption<LengthRange>[] = [
    { value: ANY_OPTION, label: words.any, choice: {} },
    ...lengthPresets.map((entry) => ({
      value: entry.key,
      label: words[entry.key],
      choice: entry.range,
    })),
  ];
  const range: LengthRange = {
    minDuration: current.minDuration,
    maxDuration: current.maxDuration,
  };
  if (range.minDuration === undefined && range.maxDuration === undefined) {
    return { options, value: ANY_OPTION };
  }
  const preset = lengthPresets.find((entry) => sameRange(entry.range, range));
  if (preset !== undefined) return { options, value: preset.key };
  options.push({ value: CUSTOM_OPTION, label: customLengthLabel(range), choice: range });
  return { options, value: CUSTOM_OPTION };
}
