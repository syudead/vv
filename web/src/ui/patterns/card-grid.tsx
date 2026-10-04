import type { CSSProperties, ReactNode } from "react";

// カードのグリッド（区画）。列の数は幅とカードの大きさ（card-0〜card-3）で決まり、
// カードは列の幅いっぱいに伸びるので、左右の端がツールバーと揃う。狭い幅では 1 列。
// 規則は web/registry/rules/patterns.md の Card grid。

export type CardSize = 0 | 1 | 2 | 3;

export interface CardGridProps {
  /** カードの最小幅の段。ライブラリの表示倍率に当たる。 */
  size?: CardSize;
  /** カード。各子が 1 枡になる。 */
  children: ReactNode;
}

export function CardGrid({ size = 1, children }: CardGridProps) {
  // 列の定義は名前つきの段（--spacing-card-*）から作る。任意の値のクラスは書けないので
  // style で渡す。
  const style: CSSProperties = {
    gridTemplateColumns: `repeat(auto-fill, minmax(min(var(--spacing-card-${String(size)}), 100%), 1fr))`,
  };
  return (
    <div data-slot="card-grid" className="grid gap-3" style={style}>
      {children}
    </div>
  );
}
