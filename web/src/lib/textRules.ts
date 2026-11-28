// Match the server's Unicode White_Space: remove U+0085 and preserve U+FEFF, unlike trim().
const edgeWhiteSpace = /^\p{White_Space}+|\p{White_Space}+$/gu;

// Reject all C0/C1 control characters (Unicode category Cc), as the server does.
const controlCharPattern = /\p{Cc}/u;

/** Trim the edges as the server's text rules do. */
export function trimWhiteSpace(text: string): string {
  return text.replace(edgeWhiteSpace, "");
}

export function hasControlCharacters(text: string): boolean {
  return controlCharPattern.test(text);
}

/** Count the code points the server counts after trimming, not UTF-16 units. */
export function trimmedCodePoints(text: string): number {
  return Array.from(trimWhiteSpace(text)).length;
}
