import {
  defaultRangeExtractor,
  type Range,
  useWindowVirtualizer,
} from "@tanstack/react-virtual";
import {
  type CSSProperties,
  type FocusEvent,
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import type { Zoom } from "../preferences/viewPreferences";
import { cardWidth } from "./Grid";

/** カードの間（`gap-2.5`）。行の間も同じ。 */
const GAP_PX = 10;
/** 画面の外に描いておく行の数（上下それぞれ）。 */
const OVERSCAN_ROWS = 2;
/** `sm` の幅。これ未満ではどの倍率でも 1 列の全幅にする（Grid と同じ）。 */
const SM_QUERY = "(min-width: 40rem)";
/** カードの題名とタグの行の高さの見積り（測るまでの間だけ使う）。 */
const TEXT_ESTIMATE_PX = 72;

/**
 * ListAnchor は一覧の位置の目印である。画面上端（上部バーの下）に最も近い項目の鍵と、
 * その上端の上部バーの下からのずれ（px）を持つ。列の数が変わっても項目の鍵で
 * 探し直せるので、戻る・表示倍率・窓の幅の変化のどれでも同じ項目を上端へ戻せる。
 */
export interface ListAnchor {
  key: string;
  offset: number;
}

/** VirtualGridHandle は一覧を持つ画面が位置の目印を読み書きする口である。 */
export interface VirtualGridHandle {
  /** 今の目印。項目が描かれていなければ undefined。 */
  anchor: () => ListAnchor | undefined;
  /** 目印の項目を、目印を取ったときと同じ高さへ戻す。見つからなければ何もしない。 */
  restore: (anchor: ListAnchor) => void;
}

/** navbarHeight は上部バーの高さ（px）である。目印のずれはこの下から測る。 */
export function navbarHeight(): number {
  return (
    parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue("--spacing-navbar"),
    ) || 48
  );
}

function cardPixels(zoom: Zoom): number {
  const name = cardWidth[zoom].slice("var(".length, -1);
  return (
    parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name)) ||
    [220, 280, 360, 480][zoom]!
  );
}

function isNarrow(): boolean {
  if (typeof window.matchMedia === "function")
    return !window.matchMedia(SM_QUERY).matches;
  return window.innerWidth < 640;
}

/**
 * columnsFor は、格子の幅 `width` と 1 枚の幅 `card` で、折り返しの格子が 1 行に並べる
 * 枚数を返す（`flex-wrap` と `gap-2.5` の並びと同じ）。狭い幅では常に 1 列である。
 */
export function columnsFor(width: number, card: number, narrow: boolean): number {
  if (narrow) return 1;
  const cell = Math.min(card, width);
  return Math.max(1, Math.floor((width + GAP_PX) / (cell + GAP_PX)));
}

interface Layout {
  width: number;
  columns: number;
  /** 行の高さの見積り（px）。 */
  estimate: number;
}

/**
 * VirtualGrid はカードの格子のうち、画面の近くの行だけを描く（issue 675）。
 *
 * 項目を幅から決めた列の数ずつ行に分け、行を `useWindowVirtualizer` で描く。各行は
 * これまでの `Grid` の行と同じ `flex justify-center gap-2.5` で、最後の半端な行も
 * 中央に寄る。カードの幅・間・並びは `Grid` と変わらない。
 *
 * - 末尾の `placeholders` 枚は読み込み中のカード（Skeleton）で、カードと同じ行に続く。
 * - フォーカスのある項目の行は画面の外へ出ても描き続ける。上下に描いておく行があるので、
 *   Tab は描いている範囲の端から次の行へ進み、ブラウザがその行を画面へ寄せる。
 * - レイアウトの無い環境（幅 0、jsdom）では全部を描く。
 * - 列の数や倍率が変わったときは、上端にあった項目を同じ高さへ戻す（ListAnchor）。
 */
export default function VirtualGrid({
  zoom,
  count,
  itemKey,
  renderItem,
  placeholders = 0,
  renderPlaceholder,
  initialAnchor,
  ref,
}: {
  zoom: Zoom;
  /** カードの数（読み込み中のカードは含めない）。 */
  count: number;
  /** index 番目の項目の鍵（React の key と目印に使う）。 */
  itemKey: (index: number) => string;
  renderItem: (index: number) => ReactNode;
  /** 末尾に並べる読み込み中のカードの数。 */
  placeholders?: number;
  renderPlaceholder?: () => ReactNode;
  /** 最初に描けたときに戻す位置（控えから戻ったとき）。 */
  initialAnchor?: ListAnchor;
  ref?: Ref<VirtualGridHandle>;
}) {
  const gridRef = useRef<HTMLDivElement | null>(null);
  const [layout, setLayout] = useState<Layout | null>(null);
  const [scrollMargin, setScrollMargin] = useState(0);
  const [focusedRow, setFocusedRow] = useState<number | null>(null);
  const total = count + placeholders;

  // 幅を測る。描く前（layout effect）に測り、測る前の 1 回は何も描かない。
  useLayoutEffect(() => {
    const grid = gridRef.current;
    if (grid === null) return;
    const measure = () => {
      const width = grid.getBoundingClientRect().width;
      const narrow = isNarrow();
      const card = cardPixels(zoom);
      const cell = narrow ? width : Math.min(card, width);
      const next: Layout = {
        width,
        columns: columnsFor(width, card, narrow),
        estimate: Math.round((cell * 9) / 16) + TEXT_ESTIMATE_PX,
      };
      setLayout((current) =>
        current !== null &&
        current.width === next.width &&
        current.columns === next.columns &&
        current.estimate === next.estimate
          ? current
          : next,
      );
      setScrollMargin(grid.getBoundingClientRect().top + window.scrollY);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(grid);
    // 格子の上の要素（見出し・絞り込みの行）の高さが変わると格子の上端も動く。
    observer.observe(document.body);
    return () => observer.disconnect();
  }, [zoom]);

  const virtualized = layout !== null && layout.width > 0;
  const columns = virtualized ? layout.columns : 1;
  const rowCount = virtualized ? Math.ceil(total / columns) : 0;

  const rangeExtractor = useCallback(
    (range: Range) => {
      const indexes = defaultRangeExtractor(range);
      if (
        focusedRow === null ||
        focusedRow >= range.count ||
        indexes.includes(focusedRow)
      )
        return indexes;
      return [...indexes, focusedRow].sort((a, b) => a - b);
    },
    [focusedRow],
  );

  const estimate = layout?.estimate ?? 240;
  const virtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => estimate,
    overscan: OVERSCAN_ROWS,
    gap: GAP_PX,
    scrollMargin,
    rangeExtractor,
    enabled: virtualized,
  });

  // 見積りが変わったら（倍率・幅）、測った高さを捨てて測り直す。
  useLayoutEffect(() => {
    virtualizer.measure();
  }, [virtualizer, estimate, columns]);

  const anchor = useCallback((): ListAnchor | undefined => {
    const grid = gridRef.current;
    if (grid === null) return undefined;
    const top = navbarHeight();
    const cells = Array.from(grid.querySelectorAll<HTMLElement>("[data-grid-key]"));
    for (const cell of cells) {
      const rect = cell.getBoundingClientRect();
      if (rect.bottom > top) {
        return { key: cell.dataset.gridKey!, offset: rect.top - top };
      }
    }
    return undefined;
  }, []);

  const keyIndex = useRef(itemKey);
  keyIndex.current = itemKey;
  const countRef = useRef(count);
  countRef.current = count;
  const columnsRef = useRef(columns);
  columnsRef.current = columns;

  const restore = useCallback(
    (target: ListAnchor) => {
      let index = -1;
      for (let i = 0; i < countRef.current; i++) {
        if (keyIndex.current(i) === target.key) {
          index = i;
          break;
        }
      }
      if (index < 0) return;
      const row = Math.floor(index / columnsRef.current);
      const top = navbarHeight();
      if (!virtualizer.options.enabled) {
        const cell = gridRef.current?.querySelector(
          `[data-grid-key="${CSS.escape(target.key)}"]`,
        );
        if (cell === null || cell === undefined) return;
        const y = window.scrollY + cell.getBoundingClientRect().top - top - target.offset;
        window.scrollTo({ top: Math.max(y, 0), behavior: "auto" });
        return;
      }
      const start = virtualizer.getOffsetForIndex(row, "start")?.[0];
      if (start === undefined) return;
      window.scrollTo({
        top: Math.max(start - top - target.offset, 0),
        behavior: "auto",
      });
    },
    [virtualizer],
  );

  useImperativeHandle(ref, () => ({ anchor, restore }), [anchor, restore]);

  // 上端の項目を、スクロールのたびに覚えておく。列の数・倍率が変わった直後に戻す先である。
  const lastAnchor = useRef<ListAnchor | undefined>(undefined);
  useEffect(() => {
    let frame = 0;
    const onScroll = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        lastAnchor.current = window.scrollY > 0 ? anchor() : undefined;
      });
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", onScroll);
    };
  }, [anchor]);

  // 列の数・倍率が変わったら、上端にあった項目を同じ高さへ戻す。
  const shape = `${String(zoom)}:${String(columns)}`;
  const knownShape = useRef(shape);
  useLayoutEffect(() => {
    if (knownShape.current === shape) return;
    knownShape.current = shape;
    const target = lastAnchor.current;
    if (target !== undefined) restore(target);
  }, [restore, shape]);

  // 控えから戻ったときの位置。描けるようになった最初の 1 回だけ戻す。
  const pendingAnchor = useRef(initialAnchor);
  useLayoutEffect(() => {
    const target = pendingAnchor.current;
    if (target === undefined || layout === null || count === 0) return;
    pendingAnchor.current = undefined;
    restore(target);
    // 測った高さで上の行がずれた分を、描いた後にもう一度合わせる。
    requestAnimationFrame(() => restore(target));
  }, [count, layout, restore]);

  const onFocus = (event: FocusEvent<HTMLDivElement>) => {
    const cell = (event.target as Element).closest<HTMLElement>("[data-grid-row]");
    if (cell === null) return;
    setFocusedRow(Number(cell.dataset.gridRow));
  };
  const onBlur = (event: FocusEvent<HTMLDivElement>) => {
    const next = event.relatedTarget;
    if (next instanceof Node && event.currentTarget.contains(next)) return;
    setFocusedRow(null);
  };

  const cell = (index: number, row: number) =>
    index < count ? (
      <div
        key={itemKey(index)}
        data-grid-key={itemKey(index)}
        data-grid-row={row}
        className="grid w-full sm:w-[min(var(--card),100%)]"
      >
        {renderItem(index)}
      </div>
    ) : (
      <div
        key={`placeholder-${String(index - count)}`}
        data-grid-row={row}
        className="grid w-full sm:w-[min(var(--card),100%)]"
      >
        {renderPlaceholder?.()}
      </div>
    );

  const style = { "--card": cardWidth[zoom] } as CSSProperties;

  if (!virtualized) {
    return (
      <div
        ref={gridRef}
        style={style}
        onFocus={onFocus}
        onBlur={onBlur}
        className="flex flex-wrap justify-center gap-2.5"
      >
        {layout !== null &&
          Array.from({ length: total }, (_, index) => cell(index, index))}
      </div>
    );
  }

  return (
    <div
      ref={gridRef}
      style={{ ...style, height: virtualizer.getTotalSize(), position: "relative" }}
      onFocus={onFocus}
      onBlur={onBlur}
    >
      {virtualizer.getVirtualItems().map((item) => {
        const first = item.index * columns;
        const last = Math.min(first + columns, total);
        return (
          <div
            key={item.key}
            ref={virtualizer.measureElement}
            data-index={item.index}
            className="absolute inset-x-0 top-0 flex justify-center gap-2.5"
            style={{ transform: `translateY(${String(item.start - scrollMargin)}px)` }}
          >
            {Array.from({ length: last - first }, (_, offset) =>
              cell(first + offset, item.index),
            )}
          </div>
        );
      })}
    </div>
  );
}
