/**
 * scrollIntoStrip は横に送る帯（overflow-x の要素）の scrollLeft を、item が帯の
 * 中央に来る位置へ合わせる。帯の左端より前には送らない。位置は描いた箱から測るので、
 * 帯が item の offsetParent でなくてもよい。
 */
export function scrollIntoStrip(root: HTMLElement, item: HTMLElement): void {
  const rootBox = root.getBoundingClientRect();
  const itemBox = item.getBoundingClientRect();
  const offset = itemBox.left - rootBox.left - root.clientLeft + root.scrollLeft;
  root.scrollLeft = Math.max(0, offset - (root.clientWidth - itemBox.width) / 2);
}
