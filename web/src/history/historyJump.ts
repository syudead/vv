import { formatMonth, formatWeekdayDate, t, type UiText } from "../i18n";

// Keep the latest 14 calendar days separate; group older dates by month.

/** Includes today. */
export const RECENT_DAYS = 14;

export interface JumpTarget {
  value: string;
  label: UiText;
}

export interface JumpTargets {
  days: JumpTarget[];
  months: JumpTarget[];
}

function localDay(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (match === null) return null;
  return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
}

function sameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

export function jumpTargets(days: readonly string[], now: Date): JumpTargets {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1);
  const oldestRecent = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate() - (RECENT_DAYS - 1),
  );
  const result: JumpTargets = { days: [], months: [] };
  const months = new Set<string>();
  for (const value of days) {
    const day = localDay(value);
    if (day === null) continue;
    if (day.getTime() >= oldestRecent.getTime()) {
      const label = sameDay(day, today)
        ? t.history.day.today
        : sameDay(day, yesterday)
          ? t.history.day.yesterday
          : formatWeekdayDate(day, now);
      result.days.push({ value, label });
      continue;
    }
    const month = value.slice(0, 7);
    if (months.has(month)) continue;
    months.add(month);
    result.months.push({ value: month, label: formatMonth(day, now) });
  }
  return result;
}

export function firstTarget(targets: JumpTargets): string | undefined {
  return (targets.days[0] ?? targets.months[0])?.value;
}
