import type { WatchHistoryEntry } from "../api/history";
import { formatMonthDay, formatWeekday, t, type UiText } from "../i18n";

// Day boundaries use midnight in the viewer's browser timezone.

/** Preserve newest-first API order within each day. */
export interface HistoryDay {
  /** Local midnight. */
  day: Date;
  key: string;
  entries: WatchHistoryEntry[];
}

function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function dayKey(date: Date): string {
  return `${String(date.getFullYear())}-${String(date.getMonth() + 1)}-${String(date.getDate())}`;
}

/**
 * Merge same-day entries across page boundaries, retaining first-seen day order; skip invalid
 * timestamps.
 */
export function groupByDay(entries: readonly WatchHistoryEntry[]): HistoryDay[] {
  const days = new Map<string, HistoryDay>();
  for (const entry of entries) {
    const played = new Date(entry.playedAt);
    if (Number.isNaN(played.getTime())) continue;
    const key = dayKey(played);
    const group = days.get(key);
    if (group === undefined) {
      days.set(key, { day: startOfDay(played), key, entries: [entry] });
    } else {
      group.entries.push(entry);
    }
  }
  return Array.from(days.values());
}

export interface DayLabel {
  name: UiText;
  /** Include the year only outside the current year. */
  date: UiText;
}

export function dayLabel(day: Date, now: Date): DayLabel {
  const today = startOfDay(now);
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1);
  const name =
    dayKey(day) === dayKey(today)
      ? t.history.day.today
      : dayKey(day) === dayKey(yesterday)
        ? t.history.day.yesterday
        : formatWeekday(day);
  return { name, date: formatMonthDay(day, now) };
}

export function msUntilNextMidnight(now: Date): number {
  const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  return Math.max(0, next.getTime() - now.getTime());
}
