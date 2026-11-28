import { t, type UiText } from "../i18n";
import { hasControlCharacters, trimmedCodePoints } from "./textRules";

export const creditNameMaxLength = 200;

/** Suppress empty-input errors while typing; CreditCommand reports them on Enter. */
export function creditNameReason(raw: string): UiText | null {
  if (hasControlCharacters(raw)) return t.creditName.controlCharacters;
  if (trimmedCodePoints(raw) > creditNameMaxLength) {
    return t.creditName.tooLong(creditNameMaxLength);
  }
  return null;
}
