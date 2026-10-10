import { decorated, LOCALE } from "./intl";
import { t } from "./messages";
import { asUiText, type UiText } from "./uiText";

export { formatNumber, selectPlural, type PluralForms } from "./intl";

// 日時の書式はカタログと同じロケール（LOCALE）の Intl で作る。`ja-JP` やロケール引数の無い
// toLocale*String は i18n/ の外に置かない（specs/023-english-i18n/research.md R-2・R-3）。

const dateFormat = new Intl.DateTimeFormat(LOCALE, { dateStyle: "medium" });
const dateTimeFormat = new Intl.DateTimeFormat(LOCALE, {
  dateStyle: "medium",
  timeStyle: "short",
});
const timeFormat = new Intl.DateTimeFormat(LOCALE, { timeStyle: "short" });
const weekdayFormat = new Intl.DateTimeFormat(LOCALE, { weekday: "long" });
const monthDayFormat = new Intl.DateTimeFormat(LOCALE, {
  month: "short",
  day: "numeric",
});
const monthDayYearFormat = new Intl.DateTimeFormat(LOCALE, {
  month: "short",
  day: "numeric",
  year: "numeric",
});
const weekdayDateFormat = new Intl.DateTimeFormat(LOCALE, {
  weekday: "short",
  month: "short",
  day: "numeric",
});
const weekdayDateYearFormat = new Intl.DateTimeFormat(LOCALE, {
  weekday: "short",
  month: "short",
  day: "numeric",
  year: "numeric",
});
const monthFormat = new Intl.DateTimeFormat(LOCALE, { month: "long" });
const monthYearFormat = new Intl.DateTimeFormat(LOCALE, {
  month: "long",
  year: "numeric",
});
const relativeFormat = new Intl.RelativeTimeFormat(LOCALE, { numeric: "always" });

const empty = asUiText("");

function toDate(value: string | Date): Date | null {
  const date = typeof value === "string" ? new Date(value) : value;
  return Number.isNaN(date.getTime()) ? null : date;
}

/** formatDate は日付を表す（例: Sep 27, 2026）。読めない値は空。 */
export function formatDate(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(dateFormat.format(date));
}

/** formatDateTime は日時を表す（例: Sep 27, 2026, 3:04 PM）。読めない値は空。 */
export function formatDateTime(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(dateTimeFormat.format(date));
}

/** formatTime は時刻を表す（例: 3:04 PM）。読めない値は空。 */
export function formatTime(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(timeFormat.format(date));
}

/** formatWeekday は曜日の名前を表す（例: Tuesday）。読めない値は空。 */
export function formatWeekday(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(weekdayFormat.format(date));
}

/**
 * formatMonthDay は月と日を表す（例: Oct 7）。`now` と年が違えば年も付ける（例: Oct 7, 2025）。
 * 読めない値は空。now は検査のために差し替えられる。
 */
export function formatMonthDay(value: string | Date, now: Date = new Date()): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const format =
    date.getFullYear() === now.getFullYear() ? monthDayFormat : monthDayYearFormat;
  return decorated(format.format(date));
}

/**
 * formatWeekdayDate は曜日と月と日を表す（例: Tue, Oct 7）。`now` と年が違えば年も付ける
 * （例: Tue, Oct 7, 2025）。読めない値は空。
 */
export function formatWeekdayDate(value: string | Date, now: Date = new Date()): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const format =
    date.getFullYear() === now.getFullYear() ? weekdayDateFormat : weekdayDateYearFormat;
  return decorated(format.format(date));
}

/**
 * formatMonth は月の名前を表す（例: September）。`now` と年が違えば年も付ける
 * （例: December 2025）。読めない値は空。
 */
export function formatMonth(value: string | Date, now: Date = new Date()): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const format = date.getFullYear() === now.getFullYear() ? monthFormat : monthYearFormat;
  return decorated(format.format(date));
}

/**
 * formatRelative は「3 days ago」のような相対表記である。区切り（たった今・分・時間・日・
 * 週・か月・年）は英語化の前と同じにする。now は検査のために差し替えられる。
 */
export function formatRelative(iso: string, now: Date = new Date()): UiText {
  const then = toDate(iso);
  if (then === null) return empty;
  const diffSec = Math.round((now.getTime() - then.getTime()) / 1000);
  if (diffSec < 45) return t.time.justNow;
  const ago = (value: number, unit: Intl.RelativeTimeFormatUnit) =>
    decorated(relativeFormat.format(-value, unit));
  const minutes = Math.round(diffSec / 60);
  if (minutes < 60) return ago(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (hours < 24) return ago(hours, "hour");
  const days = Math.round(hours / 24);
  if (days < 7) return ago(days, "day");
  const weeks = Math.round(days / 7);
  if (weeks < 5) return ago(weeks, "week");
  const months = Math.round(days / 30);
  if (months < 12) return ago(months, "month");
  return ago(Math.round(days / 365), "year");
}
