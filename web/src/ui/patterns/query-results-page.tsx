import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// A query beside the results it found; see web/registry/rules/patterns.md, Query results page.

export interface QueryResultsPageProps {
  header: ReactNode;
  /** The query's panel: a column at the start from lg, above the results below. Omit while there is none. */
  query?: ReactNode;
  /** Names the query column for screen readers. Required with `query`. */
  queryLabel?: string;
  /** Below lg, the query is one collapsed bar that sticks under the top bar; expanded, it scrolls with the page. */
  queryCollapsed?: boolean;
  /** The results: a status line and `ResultBand`s, or one state block. */
  children: ReactNode;
}

export function QueryResultsPage({
  header,
  query,
  queryLabel,
  queryCollapsed = false,
  children,
}: QueryResultsPageProps) {
  return (
    <div
      data-slot="query-results-page"
      className="flex min-h-full flex-col gap-3 p-3 sm:p-4"
    >
      {header}
      <div className="flex flex-1 flex-col gap-3 lg:flex-row lg:items-start lg:gap-6">
        {query !== undefined && (
          <aside
            data-slot="query-results-page-query"
            aria-label={queryLabel}
            className={cn(
              "min-w-0 lg:sticky lg:top-navbar lg:max-h-list-aside-max lg:w-query-aside lg:shrink-0 lg:overflow-y-auto",
              queryCollapsed && "sticky top-navbar z-10",
            )}
          >
            {query}
          </aside>
        )}
        <div
          data-slot="query-results-page-body"
          className="flex w-full min-w-0 flex-1 flex-col gap-3"
        >
          {children}
        </div>
      </div>
    </div>
  );
}

export interface ResultBandProps {
  /** The band's name, its `h2`. */
  heading: ReactNode;
  /** A quieter second part after a `·`, such as the band's range. */
  detail?: ReactNode;
  /** The band's row count, at the heading line's end. */
  count?: ReactNode;
  /** `ResultRow`s. */
  children: ReactNode;
}

/** A band of rows under a heading; consecutive bands stand gap-6 apart. */
export function ResultBand({ heading, detail, count, children }: ResultBandProps) {
  return (
    <section
      data-slot="result-band"
      className="flex min-w-0 flex-col gap-2 [[data-slot=result-band]+&]:mt-3"
    >
      <div className="flex min-w-0 items-baseline gap-2">
        <h2 className="flex min-w-0 flex-1 items-baseline gap-1 text-sm font-semibold">
          <span className="truncate">{heading}</span>
          {detail !== undefined && (
            <span className="shrink-0 text-xs font-normal text-muted-foreground">
              <span aria-hidden="true">· </span>
              {detail}
            </span>
          )}
        </h2>
        {count !== undefined && (
          <span
            data-slot="result-band-count"
            className="shrink-0 text-xs text-muted-foreground tabular-nums"
          >
            {count}
          </span>
        )}
      </div>
      <ul data-slot="result-band-rows" className="flex min-w-0 flex-col gap-3">
        {children}
      </ul>
    </section>
  );
}
