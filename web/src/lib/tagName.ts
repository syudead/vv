import { t, type UiText } from "../i18n";
import { hasControlCharacters, trimmedCodePoints } from "./textRules";

export const newlinePattern = /[\r\n]/;

export const tagNameMaxLength = 100;

/** Suppress empty-input errors while typing; all tag-name inputs share this validation. */
export function nameReason(raw: string): UiText | null {
  if (hasControlCharacters(raw)) return t.tagName.controlCharacters;
  const length = trimmedCodePoints(raw);
  if (length > tagNameMaxLength) return t.tagName.tooLong(tagNameMaxLength, length);
  return null;
}
