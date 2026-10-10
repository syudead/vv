import { formatMonth, formatWeekdayDate, t, type UiText } from "../i18n";

// 日付へ移る一覧の項目（specs/043-watch-history/ui-design.md「Jump to date」）。
// API が返す件のある日（ブラウザの時間帯の `YYYY-MM-DD`、新しい順）を、直近 14 日は日、
// それより前は月にまとめる。

/** RECENT_DAYS は日のまま並べる日数（今日を含む）である。 */
export const RECENT_DAYS = 14;

/** JumpTarget は移り先の 1 つである。 */
export interface JumpTarget {
  /** URL の `date` に書く値（`YYYY-MM-DD` か `YYYY-MM`）。 */
  value: string;
  label: UiText;
}

/** JumpTargets は区切りで分ける日と月の項目である。どちらも新しい順。 */
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

/**
 * jumpTargets は日の一覧を移り先にする。今日から数えて RECENT_DAYS 日の内の日は日の項目、
 * それより前の日はその月の項目に 1 つにまとめる。読めない日は飛ばす。
 */
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

/** firstTarget は一覧の最初の項目（いちばん新しい移り先）の値である。無ければ undefined。 */
export function firstTarget(targets: JumpTargets): string | undefined {
  return (targets.days[0] ?? targets.months[0])?.value;
}
