import { t, type UiText } from "../i18n";

export const newlinePattern = /[\r\n]/;
// Reject all C0/C1 control characters (Unicode category Cc).
const controlCharPattern = /\p{Cc}/u;

const tagNameMaxLength = 100;

/** Count trimmed Unicode code points, not UTF-16 units. */
function codePointLength(value: string): number {
  return Array.from(value.trim()).length;
}

/** Suppress empty-input errors while typing; all tag-name inputs share this validation. */
export function nameReason(raw: string): UiText | null {
  if (controlCharPattern.test(raw)) return t.tagName.controlCharacters;
  const length = codePointLength(raw);
  if (length > tagNameMaxLength) return t.tagName.tooLong(tagNameMaxLength, length);
  return null;
}
