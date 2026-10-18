import {
  createContext,
  type CSSProperties,
  type ReactNode,
  useContext,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

// See web/registry/rules/patterns.md, Card grid.

export type CardSize = 0 | 1 | 2 | 3;

export interface CardGridProps {
  size?: CardSize;
  children: ReactNode;
}

const GridColumns = createContext(1);

/** Span only existing tracks, including in narrow sidebars and at larger zoom levels. */
export function CardGridItem({
  children,
  span = 1,
}: {
  children: ReactNode;
  span?: 1 | 2;
}) {
  const columns = useContext(GridColumns);
  return (
    <div
      data-slot="card-grid-item"
      className="min-w-0"
      style={{ gridColumn: `span ${String(Math.min(span, columns))}` }}
    >
      {children}
    </div>
  );
}

export function CardGrid({ size = 1, children }: CardGridProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(1);
  useLayoutEffect(() => {
    const node = ref.current;
    if (node === null) return;
    const measure = () => {
      const tracks = getComputedStyle(node).gridTemplateColumns.match(/[\d.]+px/g);
      setColumns(Math.max(1, tracks?.length ?? 0));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [size]);
  // Pass token-derived column sizes through style; arbitrary value classes are prohibited.
  const style: CSSProperties = {
    gridTemplateColumns: `repeat(auto-fill, minmax(min(var(--spacing-card-${String(size)}), 100%), 1fr))`,
  };
  return (
    <GridColumns value={columns}>
      <div
        ref={ref}
        data-slot="card-grid"
        className="grid items-start gap-3"
        style={style}
      >
        {children}
      </div>
    </GridColumns>
  );
}
