import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// カードのグリッド（区画）。カードは表示倍率の幅（card-0〜card-3）で、行の中央に寄せて
// 折り返す。sm より狭い幅では倍率によらず 1 列の全幅にする（どの倍率でも 2 枚は並ばない
// ため）。間隔は card-gap。規則は web/registry/rules/patterns.md の Sections。

export type CardSize = 0 | 1 | 2 | 3;

export interface CardGridProps {
  /** カードの幅の段。ライブラリの表示倍率に当たる。 */
  size?: CardSize;
  /** カード。各子が 1 枡になる。 */
  children: ReactNode;
}

// クラスは静的な文字列でなければ生成されないので、段ごとに書き出す。
const cardWidth: Record<CardSize, string> = {
  0: "sm:*:w-card-0",
  1: "sm:*:w-card-1",
  2: "sm:*:w-card-2",
  3: "sm:*:w-card-3",
};

export function CardGrid({ size = 1, children }: CardGridProps) {
  return (
    <div
      data-slot="card-grid"
      className={cn(
        "flex flex-wrap justify-center gap-card-gap *:w-full sm:*:max-w-full",
        cardWidth[size],
      )}
    >
      {children}
    </div>
  );
}

/**
 * cardFrameClass は CardGrid の枡に入れるカード（動画・まとめ・フォルダ）の箱である
 * （docs/design-docs/library-ui.md「Cards」）。card の地に border の縁を付けた rounded-lg の
 * 箱で、サムネイルは上端いっぱいに、題名は箱の中のサムネイルの下に置く。hover で少し浮き、選択中は primary の縁と輪で囲む。
 * 角でサムネイルを切るので、中のリンクのフォーカスの輪は箱の外側に出す。
 */
export function cardFrameClass(selected = false): string {
  return cn(
    "group relative flex min-w-0 flex-col overflow-hidden rounded-lg border border-border bg-card text-card-foreground transition duration-200 ease-out-quart",
    "has-[a:focus-visible]:outline-2 has-[a:focus-visible]:outline-offset-2 has-[a:focus-visible]:outline-ring",
    "hover:-translate-y-0.5 hover:shadow-card-hover",
    // 装飾的な動きは動きを減らす設定で止める（library-ui.md 4）。影の最終状態は残す。
    "motion-reduce:transition-none motion-reduce:hover:translate-y-0",
    selected && "border-primary ring-2 ring-primary",
  );
}

/** cardLinkClass はカードの中のリンクである。フォーカスの輪は箱（cardFrameClass）が出す。 */
export const cardLinkClass = "flex min-w-0 flex-1 flex-col outline-none";

/** cardThumbnailClass はカードの中のサムネイルの枠で、角丸は箱に任せる。 */
export const cardThumbnailClass = "rounded-none";
