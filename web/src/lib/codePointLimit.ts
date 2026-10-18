/**
 * Count code points like the server, unlike HTML maxLength's UTF-16 units; trim only inserted text
 * to preserve the existing suffix. caret is a post-input UTF-16 offset that bounds the unchanged
 * prefix and suffix.
 */
export function limitCodePoints(
  next: string,
  max: number,
  previous = "",
  caret?: number,
): string {
  const after = Array.from(next);
  if (after.length <= max) return next;
  const before = Array.from(previous);
  // Without a caret, leave common-prefix and suffix detection unbounded.
  const cursor =
    caret === undefined ? undefined : Array.from(next.slice(0, caret)).length;
  let prefix = 0;
  while (
    prefix < before.length &&
    prefix < (cursor ?? after.length) &&
    before[prefix] === after[prefix]
  ) {
    prefix++;
  }
  let suffix = 0;
  while (
    suffix < before.length - prefix &&
    suffix < after.length - Math.max(prefix, cursor ?? prefix) &&
    before[before.length - 1 - suffix] === after[after.length - 1 - suffix]
  ) {
    suffix++;
  }
  const inserted = after.slice(prefix, after.length - suffix);
  const room = Math.max(max - prefix - suffix, 0);
  return [
    ...after.slice(0, prefix),
    ...inserted.slice(0, room),
    ...after.slice(after.length - suffix),
  ]
    .slice(0, max)
    .join("");
}
