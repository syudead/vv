import type { VirtualItem } from "@tanstack/react-virtual";
import type { ReactNode } from "react";

/** Use spacer rows instead of absolute positioning so table columns remain aligned. */
export function virtualBodyRows({
  items,
  scrollStart,
  totalSize,
  columnCount,
  renderRow,
}: {
  items: readonly VirtualItem[];
  scrollStart: number;
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

/** Preserve unrendered height without exposing spacers to assistive technology. */
function SpacerRow({ height, columnCount }: { height: number; columnCount: number }) {
  return (
    <tr aria-hidden="true" style={{ height }}>
      <td colSpan={columnCount} className="p-0" />
    </tr>
  );
}
