import { decorated, LOCALE } from "./intl";
import { t } from "./messages";
import { asUiText, type UiText } from "./uiText";

export {
  formatDecimal,
  formatLanguage,
  formatNumber,
  selectPlural,
  type PluralForms,
} from "./intl";

// Use catalog LOCALE for Intl formatting; keep locale-specific calls inside i18n/.

const dateFormat = new Intl.DateTimeFormat(LOCALE, { dateStyle: "medium" });
const dateTimeFormat = new Intl.DateTimeFormat(LOCALE, {
  dateStyle: "medium",
  timeStyle: "short",
});
const timeFormat = new Intl.DateTimeFormat(LOCALE, { timeStyle: "short" });
const secondsFormat = new Intl.DateTimeFormat(LOCALE, { timeStyle: "medium" });
// timeStyle cannot be combined with fractionalSecondDigits, so the fields are spelled out.
const millisecondsFormat = new Intl.DateTimeFormat(LOCALE, {
  hour: "numeric",
  minute: "2-digit",
  second: "2-digit",
  fractionalSecondDigits: 3,
});
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

/** Return empty for invalid dates. */
export function formatDate(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(dateFormat.format(date));
}

/** Return empty for invalid timestamps. */
export function formatDateTime(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(dateTimeFormat.format(date));
}

/**
 * The time of day with seconds, and with milliseconds when the timestamp's fraction of a second has
 * any; empty for an invalid timestamp.
 */
export function formatPreciseTime(value: string): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const fraction = /\.(\d+)/.exec(value)?.[1] ?? "";
  const format = /[1-9]/.test(fraction.slice(0, 3)) ? millisecondsFormat : secondsFormat;
  return decorated(format.format(date));
}

/** Return empty for invalid times. */
export function formatTime(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(timeFormat.format(date));
}

/** Return empty for invalid dates. */
export function formatWeekday(value: string | Date): UiText {
  const date = toDate(value);
  return date === null ? empty : decorated(weekdayFormat.format(date));
}

/** Include the year only outside now's year; invalid dates return empty. */
export function formatMonthDay(value: string | Date, now: Date = new Date()): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const format =
    date.getFullYear() === now.getFullYear() ? monthDayFormat : monthDayYearFormat;
  return decorated(format.format(date));
}

/** Include the year only outside now's year; invalid dates return empty. */
export function formatWeekdayDate(value: string | Date, now: Date = new Date()): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const format =
    date.getFullYear() === now.getFullYear() ? weekdayDateFormat : weekdayDateYearFormat;
  return decorated(format.format(date));
}

/** Include the year only outside now's year; invalid dates return empty. */
export function formatMonth(value: string | Date, now: Date = new Date()): UiText {
  const date = toDate(value);
  if (date === null) return empty;
  const format = date.getFullYear() === now.getFullYear() ? monthFormat : monthYearFormat;
  return decorated(format.format(date));
}

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
