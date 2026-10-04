import type { ReactNode } from "react";

import { Table } from "@/ui/shadcn/table";

// 表（区画）。Table をカードの面と枠に入れる。中身は TableHeader と TableBody で書き、
// 行の高さと余白は Table が持つ。規則は web/registry/rules/patterns.md の Data table。

export interface DataTableProps {
  /** 表の名前。支援技術に読まれる。 */
  label: string;
  /** TableHeader と TableBody。 */
  children: ReactNode;
}

export function DataTable({ label, children }: DataTableProps) {
  return (
    <div
      data-slot="data-table"
      className="overflow-hidden rounded-md border border-border bg-card text-card-foreground"
    >
      <Table aria-label={label}>{children}</Table>
    </div>
  );
}
