/** Reserve space for +N, but always show at least one tag, truncated if needed. CardTagRow owns measurement. */
export function computeVisibleTagCount(
  tagWidths: readonly number[],
  overflowWidth: number,
  gap: number,
  available: number,
): number {
  if (available <= 0) return tagWidths.length;

  const totalWidth = tagWidths.reduce(
    (sum, width, index) => sum + width + (index > 0 ? gap : 0),
    0,
  );
  if (totalWidth <= available) return tagWidths.length;

  let used = 0;
  let visible = 0;
  for (let index = 0; index < tagWidths.length; index += 1) {
    const width = tagWidths[index] ?? 0;
    const withThisTag = used + (index > 0 ? gap : 0) + width;
    const withOverflow = withThisTag + gap + overflowWidth;
    if (withOverflow > available) break;
    used = withThisTag;
    visible += 1;
  }
  if (visible === 0 && tagWidths.length > 0) return 1;
  return visible;
}
