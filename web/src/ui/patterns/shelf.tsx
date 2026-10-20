import type { ReactNode } from "react";

import { PageHeader } from "./page-header";

// One row of cards under a heading that scrolls sideways; see web/registry/rules/patterns.md, Sections.

export interface ShelfProps {
  /** The heading (`h2`). */
  title: ReactNode;
  /** The number of items, after the heading. */
  count?: ReactNode;
  /** One `ghost` `sm` action that shows every item, such as `See all`. */
  action?: ReactNode;
  /** `ShelfItem`s, or one state block. */
  children: ReactNode;
}

export function Shelf({ title, count, action, children }: ShelfProps) {
  return (
    <section data-slot="shelf" className="flex min-w-0 flex-col gap-3">
      <PageHeader title={title} headingLevel={2} count={count} actions={action} />
      <ul className="-mx-1 flex snap-x gap-3 overflow-x-auto px-1 pb-2">{children}</ul>
    </section>
  );
}

/** One card in a shelf, `card-0` wide. */
export function ShelfItem({ children }: { children: ReactNode }) {
  return <li className="w-card-0 shrink-0 snap-start">{children}</li>;
}
