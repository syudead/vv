/**
 * Measure rendered bounds so the strip need not be the item's offsetParent; clamp before the
 * strip's start.
 */
export function scrollIntoStrip(root: HTMLElement, item: HTMLElement): void {
  const rootBox = root.getBoundingClientRect();
  const itemBox = item.getBoundingClientRect();
  const offset = itemBox.left - rootBox.left - root.clientLeft + root.scrollLeft;
  root.scrollLeft = Math.max(0, offset - (root.clientWidth - itemBox.width) / 2);
}
