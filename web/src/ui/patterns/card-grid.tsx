import type { CSSProperties, ReactNode } from "react";

// See web/registry/rules/patterns.md, Card grid.

export type CardSize = 0 | 1 | 2 | 3;

/** A video card size, or face for the people grid's round face cards. */
export type CardGridSize = CardSize | "face";

export interface CardGridProps {
  size?: CardGridSize;
  children: ReactNode;
}

export function CardGrid({ size = 1, children }: CardGridProps) {
  // Pass token-derived column sizes through style; arbitrary value classes are prohibited.
  const style: CSSProperties = {
    gridTemplateColumns: `repeat(auto-fill, minmax(min(var(--spacing-card-${String(size)}), 100%), 1fr))`,
  };
  return (
    <div data-slot="card-grid" className="grid gap-3" style={style}>
      {children}
    </div>
  );
}
