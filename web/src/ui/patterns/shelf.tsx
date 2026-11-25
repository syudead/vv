import type { ReactNode } from "react";

import { PageHeader } from "./page-header";

// One kind of item under a heading with its count; see web/registry/rules/patterns.md, Sections.

export interface ShelfProps {
  /** The heading (`h2`). */
  title: ReactNode;
  /** The number of items, after the heading. */
  count?: ReactNode;
  /** Drawn at the right end of the heading row. */
  actions?: ReactNode;
  /**
   * `grid` (default): the children are a `CardGrid` of every item, then its `LoadMoreRow`, or one
   * state block. `row`: each child is a card, `card-0` wide, in one line that scrolls sideways.
   */
  layout?: "grid" | "row";
  children: ReactNode;
}

export function Shelf({ title, count, actions, layout = "grid", children }: ShelfProps) {
  return (
    <section data-slot="shelf" className="flex min-w-0 flex-col gap-3">
      <PageHeader title={title} headingLevel={2} count={count} actions={actions} />
      {layout === "row" ? (
        <div
          data-slot="shelf-row"
          // Box selection skips the cards this row clips.
          data-scroll-row=""
          className="flex min-w-0 gap-3 overflow-x-auto pb-1 *:w-card-0 *:shrink-0"
        >
          {children}
        </div>
      ) : (
        children
      )}
    </section>
  );
}
