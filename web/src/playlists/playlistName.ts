// Match server code-point limits; HTML maxLength counts UTF-16 units and rejects emoji too early.

export const playlistNameLimit = 100;

// Match server Unicode White_Space: remove U+0085 and preserve U+FEFF, unlike trim().
const edgeWhiteSpace = /^\p{White_Space}+|\p{White_Space}+$/gu;

/** Trim the edges as the server's text rules do. */
export function trimWhiteSpace(text: string): string {
  return text.replace(edgeWhiteSpace, "");
}

/** Return null for empty names or more than 100 code points. */
export function playlistName(text: string): string | null {
  const name = trimWhiteSpace(text);
  const length = Array.from(name).length;
  if (length === 0 || length > playlistNameLimit) return null;
  return name;
}
