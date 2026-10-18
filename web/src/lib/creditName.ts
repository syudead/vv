import { t, type UiText } from "../i18n";

const controlCharPattern = /\p{Cc}/u;

export const creditNameMaxLength = 200;

/** Suppress empty-input errors while typing; CreditCommand reports them on Enter. */
export function creditNameReason(raw: string): UiText | null {
  if (controlCharPattern.test(raw)) return t.creditName.controlCharacters;
  if (Array.from(raw.trim()).length > creditNameMaxLength) {
    return t.creditName.tooLong(creditNameMaxLength);
  }
  return null;
}
