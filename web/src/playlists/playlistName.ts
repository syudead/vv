import { trimWhiteSpace } from "../lib/textRules";

// Match server code-point limits; HTML maxLength counts UTF-16 units and rejects emoji too early.

export const playlistNameLimit = 100;

/** Return null for empty names or more than 100 code points. */
export function playlistName(text: string): string | null {
  const name = trimWhiteSpace(text);
  const length = Array.from(name).length;
  if (length === 0 || length > playlistNameLimit) return null;
  return name;
}
