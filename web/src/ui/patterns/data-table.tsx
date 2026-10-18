import type { CSSProperties, ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Table } from "@/ui/shadcn/table";

// See web/registry/rules/patterns.md, Data table.

export interface DataTableProps {
  label: string;
  /** Viewport offset in pixels, normally the sticky band's bottom edge. */
  stickyHeaderTop?: number;
  children: ReactNode;
}

export function DataTable({ label, stickyHeaderTop, children }: DataTableProps) {
  const sticky = stickyHeaderTop !== undefined;
  // Keep wrappers out of the scroll-container chain so headers stick to the page, not the table box.
  // Use a style variable for the measured, non-token position.
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
