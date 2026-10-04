import {
  defaultRangeExtractor,
  type Range,
  useWindowVirtualizer,
} from "@tanstack/react-virtual";
import {
  type FocusEvent,
  type ReactNode,
  type Ref,
  useCallback,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import { type ListAnchor, navbarHeight, type VirtualGridHandle } from "./VirtualGrid";

/**
 * TableRowSlot は、仮想化した `tbody` の 1 行に付ける値である。`VideoRow`・`GroupRow` は
 * これを props として受け取り、`tr` の `data-row-key`・`data-index`・`data-stripe` と `ref` にする。
 */
export interface TableRowSlot {
  rowKey: string;
  rowIndex: number;
  /** 縞を付ける行か（項目の番号が偶数）。 */
  striped: boolean;
  measureRef?: (element: HTMLTableRowElement | null) => void;
}

/** 行の高さの見積り（サムネイル `w-28` の 16:9 と上下の余白）。測るまでの間だけ使う。 */
const ROW_ESTIMATE_PX = 80;
const OVERSCAN_ROWS = 6;
/** 縞は `data-stripe` の行に付ける（以前の `nth-child(odd)` と同じ強さのセレクタ）。 */
const STRIPES = "[&>tr[data-stripe]]:bg-hover-wash/40";

/**
 * VirtualTableBody はライブラリのリスト表示の `tbody` のうち、画面の近くの行だけを描く
 * （issue 675）。描かない行の高さは上下の空の行が引き受け、表の列の幅は見出しと共有の
 * まま変わらない。縞は描いた行の並びではなく項目の番号で付ける（`nth-child` では
 * 上の空の行と描き始めの行の移動で縞が入れ替わる）。
 *
 * `renderRow(index, slot)` は `tr` を返し、`slot` の値をその `tr` に付ける（`tr` は
 * `tbody` の直下でなければならないので、ここでは包まない）。
 */
export default function VirtualTableBody({
  count,
  itemKey,
  renderRow,
  initialAnchor,
  columns,
  ref,
}: {
  count: number;
  itemKey: (index: number) => string;
  renderRow: (index: number, slot: TableRowSlot) => ReactNode;
  initialAnchor?: ListAnchor;
  /** 表の列の数（空の行の `td` に使う）。 */
  columns: number;
  ref?: Ref<VirtualGridHandle>;
}) {
  const bodyRef = useRef<HTMLTableSectionElement | null>(null);
  const [scrollMargin, setScrollMargin] = useState(0);
  const [measurable, setMeasurable] = useState<boolean | null>(null);
  const [focusedRow, setFocusedRow] = useState<number | null>(null);

  useLayoutEffect(() => {
    const body = bodyRef.current;
    if (body === null) return;
    const measure = () => {
      const rect = body.getBoundingClientRect();
      setMeasurable(rect.width > 0);
      setScrollMargin(rect.top + window.scrollY);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(document.body);
    return () => observer.disconnect();
  }, []);

  const rangeExtractor = useCallback(
    (range: Range) => {
      const indexes = defaultRangeExtractor(range);
      if (focusedRow === null || focusedRow >= range.count || indexes.includes(focusedRow))
        return indexes;
      return [...indexes, focusedRow].sort((a, b) => a - b);
    },
    [focusedRow],
  );

  const virtualized = measurable === true;
  const virtualizer = useWindowVirtualizer({
    count,
    estimateSize: () => ROW_ESTIMATE_PX,
    overscan: OVERSCAN_ROWS,
    scrollMargin,
    rangeExtractor,
    getItemKey: itemKey,
    enabled: virtualized,
  });

  const anchor = useCallback((): ListAnchor | undefined => {
    const body = bodyRef.current;
    if (body === null) return undefined;
    const top = navbarHeight();
    for (const row of Array.from(body.querySelectorAll<HTMLElement>("[data-row-key]"))) {
      const rect = row.getBoundingClientRect();
      if (rect.bottom > top) return { key: row.dataset.rowKey!, offset: rect.top - top };
    }
    return undefined;
  }, []);

  const keyRef = useRef(itemKey);
  keyRef.current = itemKey;
  const countRef = useRef(count);
  countRef.current = count;

  const restore = useCallback(
    (target: ListAnchor) => {
      let index = -1;
      for (let i = 0; i < countRef.current; i++) {
        if (keyRef.current(i) === target.key) {
          index = i;
          break;
        }
      }
      if (index < 0) return;
      const top = navbarHeight();
      if (!virtualizer.options.enabled) {
        const row = bodyRef.current?.querySelector(
          `[data-row-key="${CSS.escape(target.key)}"]`,
        );
        if (row === null || row === undefined) return;
        const y = window.scrollY + row.getBoundingClientRect().top - top - target.offset;
        window.scrollTo({ top: Math.max(y, 0), behavior: "auto" });
        return;
      }
      const start = virtualizer.getOffsetForIndex(index, "start")?.[0];
      if (start === undefined) return;
      window.scrollTo({ top: Math.max(start - top - target.offset, 0), behavior: "auto" });
    },
    [virtualizer],
  );

  useImperativeHandle(ref, () => ({ anchor, restore }), [anchor, restore]);

  const pendingAnchor = useRef(initialAnchor);
  useLayoutEffect(() => {
    const target = pendingAnchor.current;
    if (target === undefined || measurable === null || count === 0) return;
    pendingAnchor.current = undefined;
    restore(target);
    requestAnimationFrame(() => restore(target));
  }, [count, measurable, restore]);

  const onFocus = (event: FocusEvent<HTMLTableSectionElement>) => {
    const row = (event.target as Element).closest<HTMLElement>("[data-index]");
    if (row === null) return;
    setFocusedRow(Number(row.dataset.index));
  };
  const onBlur = (event: FocusEvent<HTMLTableSectionElement>) => {
    const next = event.relatedTarget;
    if (next instanceof Node && event.currentTarget.contains(next)) return;
    setFocusedRow(null);
  };

  const row = (index: number) =>
    renderRow(index, {
      rowKey: itemKey(index),
      rowIndex: index,
      striped: index % 2 === 0,
      measureRef: virtualized ? virtualizer.measureElement : undefined,
    });

  if (!virtualized) {
    return (
      <tbody ref={bodyRef} onFocus={onFocus} onBlur={onBlur} className={STRIPES}>
        {measurable !== null && Array.from({ length: count }, (_, index) => row(index))}
      </tbody>
    );
  }

  // 描く行の間の描かない行（上・下と、フォーカスのため離れて描く行との間）は、空の行が
  // 高さを引き受ける。
  const items = virtualizer.getVirtualItems();
  const rows: ReactNode[] = [];
  let edge = scrollMargin;
  const spacer = (height: number, key: string) =>
    height > 0.5 && (
      <tr key={key} aria-hidden="true">
        <td colSpan={columns} style={{ height, padding: 0, border: 0 }} />
      </tr>
    );
  for (const item of items) {
    rows.push(spacer(item.start - edge, `gap-${String(item.index)}`));
    rows.push(row(item.index));
    edge = item.end;
  }
  rows.push(spacer(scrollMargin + virtualizer.getTotalSize() - edge, "gap-end"));
  return (
    <tbody ref={bodyRef} onFocus={onFocus} onBlur={onBlur} className={STRIPES}>
      {rows}
    </tbody>
  );
}
