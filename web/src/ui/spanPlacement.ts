/** A stretch of a video, in seconds from its start. */
export interface TimeSpan {
  start: number;
  end: number;
}

/** Where a span sits on a lane, as percentages of the lane's width. */
export interface SpanPlacement {
  left: number;
  width: number;
}

/**
 * Places a span on a lane `lengthSeconds` long, clamped to the lane, so a span past the end is
 * drawn to the end. Answers null when the lane has no length or nothing of the span lies on it.
 */
export function spanPlacement(
  span: TimeSpan,
  lengthSeconds: number,
): SpanPlacement | null {
  if (!(lengthSeconds > 0)) return null;
  const start = Math.min(Math.max(span.start, 0), lengthSeconds);
  const end = Math.min(Math.max(span.end, 0), lengthSeconds);
  // Negated comparisons also drop a span with a NaN bound.
  if (!(start < lengthSeconds && end >= start)) return null;
  return {
    left: (start / lengthSeconds) * 100,
    width: ((end - start) / lengthSeconds) * 100,
  };
}
