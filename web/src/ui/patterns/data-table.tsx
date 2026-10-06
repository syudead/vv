import type { CSSProperties, ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Table } from "@/ui/shadcn/table";

// 表（区画）。Table をカードの面と枠に入れる。中身は TableHeader と TableBody で書き、
// 行の高さと余白は Table が持つ。規則は web/registry/rules/patterns.md の Data table。

export interface DataTableProps {
  /** 表の名前。支援技術に読まれる。 */
  label: string;
  /**
   * 列の見出し（TableHeader）を貼り付ける、表示域の上端からの位置（px）。管理表ページでは
   * 貼り付いた帯の下端を渡す。渡すと、行を送っても列の見出しが帯の直下に残る。
   */
  stickyHeaderTop?: number;
  /** TableHeader と TableBody。 */
  children: ReactNode;
}

export function DataTable({ label, stickyHeaderTop, children }: DataTableProps) {
  const sticky = stickyHeaderTop !== undefined;
  // 貼り付けるときは、包みと Table の入れ物をスクロールの箱にしない（overflow-clip・
  // overflow-visible）。箱の中では見出しが箱に対して貼り付き、ページの上には残らない。
  // 位置は段の外の数値なので、style の変数で渡す。
  const style: CSSProperties | undefined = sticky
    ? ({ "--data-table-head-top": `${String(stickyHeaderTop)}px` } as CSSProperties)
    : undefined;
  return (
    <div
      data-slot="data-table"
      className={cn(
        "rounded-md border border-border bg-card text-card-foreground",
        sticky
          ? "overflow-clip *:data-[slot=table-container]:overflow-visible [&_thead]:sticky [&_thead]:top-data-table-head [&_thead]:z-10 [&_thead]:bg-card [&_th]:shadow-table-head"
          : "overflow-hidden",
      )}
      style={style}
    >
      <Table aria-label={label}>{children}</Table>
    </div>
  );
}
