import { useMemo } from "react";

import { type VideoConditionParams, videoConditionNames } from "../api/client";
import { formatDate, t, type UiText } from "../i18n";

/** Duration, added-day and watch-state filters; the URL, the listing and smart-folder rules share the names. */
export type VideoConditions = VideoConditionParams;

export type WatchState = NonNullable<VideoConditions["watchState"]>;

/** Chips in the library's order: duration, added date, watch state. */
export type VideoConditionKind = "duration" | "added" | "watch";

const watchStates: readonly WatchState[] = ["unwatched", "inProgress", "watched"];

const dayPattern = /^(\d{4})-(\d{2})-(\d{2})$/;

/** A local Date for a YYYY-MM-DD day, or undefined when the day does not exist. */
function localDay(value: string): Date | undefined {
  const match = dayPattern.exec(value);
  if (match === null) return undefined;
  const [year, month, day] = [Number(match[1]), Number(match[2]), Number(match[3])];
  const date = new Date(year, month - 1, day);
  return date.getFullYear() === year &&
    date.getMonth() === month - 1 &&
    date.getDate() === day
    ? date
    : undefined;
}

function firstMatching<T>(
  raw: readonly string[],
  read: (value: string) => T | undefined,
) {
  for (const value of raw) {
    const parsed = read(value);
    if (parsed !== undefined) return parsed;
  }
  return undefined;
}

const readDuration = (value: string) => (/^\d+$/.test(value) ? Number(value) : undefined);
const readDay = (value: string) => (localDay(value) === undefined ? undefined : value);
const readWatch = (value: string) => watchStates.find((state) => state === value);

/** Ignore values the server would reject, and drop both bounds of a pair given in the wrong order. */
export function parseVideoConditions(search: URLSearchParams): VideoConditions {
  let minDurationMs = firstMatching(search.getAll("minDurationMs"), readDuration);
  let maxDurationMs = firstMatching(search.getAll("maxDurationMs"), readDuration);
  let addedFrom = firstMatching(search.getAll("addedFrom"), readDay);
  let addedTo = firstMatching(search.getAll("addedTo"), readDay);
  if (
    minDurationMs !== undefined &&
    maxDurationMs !== undefined &&
    maxDurationMs < minDurationMs
  ) {
    minDurationMs = maxDurationMs = undefined;
  }
  if (addedFrom !== undefined && addedTo !== undefined && addedTo < addedFrom) {
    addedFrom = addedTo = undefined;
  }
  const watchState = firstMatching(search.getAll("watchState"), readWatch);
  return { minDurationMs, maxDurationMs, addedFrom, addedTo, watchState };
}

/** Keep identity stable across unrelated URL changes; guests get none, as with the credit filters. */
export function useVideoConditions(search: string, owner: boolean): VideoConditions {
  const { minDurationMs, maxDurationMs, addedFrom, addedTo, watchState } = owner
    ? parseVideoConditions(new URLSearchParams(search))
    : ({} as VideoConditions);
  return useMemo(
    () => ({ minDurationMs, maxDurationMs, addedFrom, addedTo, watchState }),
    [minDurationMs, maxDurationMs, addedFrom, addedTo, watchState],
  );
}

export function hasVideoConditions(conditions: VideoConditions): boolean {
  return videoConditionNames.some((name) => conditions[name] !== undefined);
}

export function videoConditionsSignature(conditions: VideoConditions): string {
  return videoConditionNames.map((name) => String(conditions[name] ?? "")).join("\u0000");
}

const paramsOf: Record<VideoConditionKind, readonly (keyof VideoConditions)[]> = {
  duration: ["minDurationMs", "maxDurationMs"],
  added: ["addedFrom", "addedTo"],
  watch: ["watchState"],
};

/** URL changes that remove one chip's condition, both bounds of a range included. */
export function withoutVideoCondition(
  kind: VideoConditionKind,
): Record<string, string[]> {
  return Object.fromEntries(paramsOf[kind].map((name) => [name, []]));
}

export const clearedVideoConditions: Readonly<Record<string, string[]>> =
  Object.fromEntries(videoConditionNames.map((name) => [name, []]));

/** The condition without one chip's fields, for a rule edited in the smart folder dialog. */
export function dropVideoCondition<T extends VideoConditions>(
  conditions: T,
  kind: VideoConditionKind,
): T {
  const next = { ...conditions };
  for (const name of paramsOf[kind]) delete next[name];
  return next;
}

/** A duration rounded to minutes, as `10 min`, `1 h` or `2 h 30 min`. */
export function durationText(durationMs: number): UiText {
  const words = t.library.conditions;
  const total = Math.round(durationMs / 60_000);
  const hours = Math.floor(total / 60);
  const minutes = total % 60;
  if (hours === 0) return words.minutes(minutes);
  if (minutes === 0) return words.hours(hours);
  return words.hoursMinutes(hours, minutes);
}

/** Callers pass days parseVideoConditions or the server accepted. */
function dayText(day: string): UiText {
  return formatDate(localDay(day) ?? day);
}

function durationChip({ minDurationMs: min, maxDurationMs: max }: VideoConditions) {
  const words = t.library.conditions;
  if (min !== undefined && max !== undefined) {
    return words.durationRange(durationText(min), durationText(max));
  }
  if (min !== undefined) return words.durationAtLeast(durationText(min));
  if (max !== undefined) return words.durationUpTo(durationText(max));
  return undefined;
}

function addedChip({ addedFrom: from, addedTo: to }: VideoConditions) {
  const words = t.library.conditions;
  if (from !== undefined && to !== undefined) {
    return words.addedRange(dayText(from), dayText(to));
  }
  if (from !== undefined) return words.addedSince(dayText(from));
  if (to !== undefined) return words.addedUntil(dayText(to));
  return undefined;
}

export function watchText(state: WatchState): UiText {
  return state === "unwatched"
    ? t.library.conditions.unwatched
    : t.list.filter.watchOptions[state];
}

export interface VideoConditionChip {
  kind: VideoConditionKind;
  text: UiText;
  /** The chip's remove control's accessible name. */
  remove: UiText;
}

/** One chip per set condition in the library's order. */
export function videoConditionChips(conditions: VideoConditions): VideoConditionChip[] {
  const words = t.library.conditions;
  const chips: VideoConditionChip[] = [];
  const duration = durationChip(conditions);
  if (duration !== undefined) {
    chips.push({ kind: "duration", text: duration, remove: words.removeDuration });
  }
  const added = addedChip(conditions);
  if (added !== undefined) {
    chips.push({ kind: "added", text: added, remove: words.removeAdded });
  }
  if (conditions.watchState !== undefined) {
    chips.push({
      kind: "watch",
      text: watchText(conditions.watchState),
      remove: words.removeWatch,
    });
  }
  return chips;
}
