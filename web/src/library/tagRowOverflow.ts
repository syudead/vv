/**
 * computeVisibleTagCount は、カードのタグの行に収まる先頭からのタグの個数を決める
 * （specs/014-video-tags/ui-design.md「Overflow」）。純粋な計算だけをここに置き、
 * 実際の幅を測る DOM の扱い（ResizeObserver・layout effect）は CardTagRow が持つ。
 *
 * すべてのタグが収まればその個数を返す（「+N」は要らない）。収まらないときは、
 * 「+N」チップ（`overflowWidth`）を置く分の余白を残して収まる個数を返す。
 * 1つも収まらないときは 0 を返す（呼び出し側は「+N」だけを出す）。
 */
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
    // このタグを出したあとも、「+N」チップを置ける余白が要る。
    const withOverflow = withThisTag + gap + overflowWidth;
    if (withOverflow > available) break;
    used = withThisTag;
    visible += 1;
  }
  return visible;
}
