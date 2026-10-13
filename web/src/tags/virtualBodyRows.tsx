import type { VirtualItem } from "@tanstack/react-virtual";
import type { ReactNode } from "react";

/**
 * virtualBodyRows は仮想化した行を表の中に描く。描かない行の高さは、行の間と前後に置く
 * 空の行で保つ（行を absolute で重ねると、表の列の幅がそろわないため）。タグの画面と
 * 人物・団体の画面が使う。`renderRow` は `index` の行を描く。
 */
export function virtualBodyRows({
  items,
  scrollStart,
  totalSize,
  columnCount,
  renderRow,
}: {
  items: readonly VirtualItem[];
  /** 仮想化の `scrollMargin`（一覧の上端の文書の中での位置）。 */
  scrollStart: number;
  /** 仮想化の `getTotalSize()`。 */
  totalSize: number;
  columnCount: number;
  renderRow: (item: VirtualItem) => ReactNode;
}): ReactNode[] {
  const rows: ReactNode[] = [];
  let cursor = scrollStart;
  for (const item of items) {
    const gap = item.start - cursor;
    if (gap > 0) {
      rows.push(
        <SpacerRow
          key={`gap-${String(item.index)}`}
          height={gap}
          columnCount={columnCount}
        />,
      );
    }
    rows.push(renderRow(item));
    cursor = item.end;
  }
  const listEnd = scrollStart + totalSize;
  if (items.length > 0 && listEnd > cursor) {
    rows.push(
      <SpacerRow key="gap-end" height={listEnd - cursor} columnCount={columnCount} />,
    );
  }
  return rows;
}

/** SpacerRow は描かない行の高さを保つ空の行である。読み上げない。 */
function SpacerRow({ height, columnCount }: { height: number; columnCount: number }) {
  return (
    <tr aria-hidden="true" style={{ height }}>
      <td colSpan={columnCount} className="p-0" />
    </tr>
  );
}
