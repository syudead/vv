import type { ReactNode } from "react";

import { PageHeader } from "./page-header";

// One kind of item under a heading with its count; see web/registry/rules/patterns.md, Sections.

export interface ShelfProps {
  /** The heading (`h2`). */
  title: ReactNode;
  /** The number of items, after the heading. */
  count?: ReactNode;
  /** A `CardGrid` of every item, then its `LoadMoreRow`; or one state block. */
  children: ReactNode;
}

export function Shelf({ title, count, children }: ShelfProps) {
  return (
    <section data-slot="shelf" className="flex min-w-0 flex-col gap-3">
      <PageHeader title={title} headingLevel={2} count={count} />
      {children}
    </section>
  );
}
