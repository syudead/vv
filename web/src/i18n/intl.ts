import { asUiText, type UiText } from "./uiText";

/** Catalog and formatters share one locale fixed at startup. */
export const LOCALE = "en-US";

type Decorate = (text: string) => string;

const identity: Decorate = (text) => text;
let decorate: Decorate = identity;

/** Pseudo-locale decoration marks formatted numbers and dates as catalog-derived in UI tests. */
export function decorated(text: string): UiText {
  return asUiText(decorate(text));
}

/** Test-only decoration override; null restores the default. */
export function setDecoration(next: Decorate | null): void {
  decorate = next ?? identity;
}

const numberFormat = new Intl.NumberFormat(LOCALE);
const pluralRules = new Intl.PluralRules(LOCALE);
const listFormat = new Intl.ListFormat(LOCALE, { type: "conjunction" });

export function formatNumber(value: number): UiText {
  return decorated(numberFormat.format(value));
}

const decimalFormats = new Map<string, Intl.NumberFormat>();

export function formatDecimal(value: number, max: number, min = 0): UiText {
  const key = `${String(min)}-${String(max)}`;
  let format = decimalFormats.get(key);
  if (format === undefined) {
    format = new Intl.NumberFormat(LOCALE, {
      minimumFractionDigits: min,
      maximumFractionDigits: max,
    });
    decimalFormats.set(key, format);
  }
  return decorated(format.format(value));
}

const languageNames = new Intl.DisplayNames(LOCALE, { type: "language" });

/**
 * Return null for empty or und language codes; preserve unknown codes verbatim and decorate
 * recognized names.
 */
export function formatLanguage(code: string): UiText | string | null {
  const trimmed = code.trim();
  if (trimmed === "" || trimmed.toLowerCase() === "und") return null;
  let name: string | undefined;
  try {
    name = languageNames.of(trimmed);
  } catch {
    return trimmed;
  }
  return name === undefined || name === trimmed ? trimmed : decorated(name);
}

export interface PluralForms<T> {
  one: T;
  other: T;
}

/** Unsupported English plural categories fall back to other. */
export function selectPlural<T>(count: number, forms: PluralForms<T>): T {
  return pluralRules.select(count) === "one" ? forms.one : forms.other;
}

/** Pass catalog text as list items. */
export function formatList(items: readonly string[]): string {
  return listFormat.format(items);
}
