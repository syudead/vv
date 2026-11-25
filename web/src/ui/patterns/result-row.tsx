import type { ReactNode } from "react";

import { Collapsible, CollapsibleContent } from "@/ui/shadcn/collapsible";

import { ExpandToggle, type ExpandToggleLabel } from "./expand-toggle";

// One candidate of a query's results on a card; see web/registry/rules/patterns.md, Sections.
// contain-inline-size makes a row take its band's width, so a long strip scrolls instead of widening the page.

export type ResultRowToggleLabel = ExpandToggleLabel;

export interface ResultRowProps {
  /** The row's figure before its text, such as a `RateRing`; never a control. */
  mark: ReactNode;
  /** Truncated before the badge and the actions. */
  title: ReactNode;
  /** Chips after the title; never truncated. */
  badge?: ReactNode;
  /** One quieter line under the title; apply truncate to its text element. */
  meta?: ReactNode;
  /** `sm` buttons before the expand button. */
  actions?: ReactNode;
  /** Shown under the header line while `expanded`. */
  children: ReactNode;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  toggleLabel: ResultRowToggleLabel;
}

export function ResultRow({
  mark,
  title,
  badge,
  meta,
  actions,
  children,
  expanded,
  onExpandedChange,
  toggleLabel,
}: ResultRowProps) {
  return (
    <Collapsible open={expanded} onOpenChange={onExpandedChange} asChild>
      <li
        data-slot="result-row"
        className="flex min-w-0 flex-col rounded-md border border-border bg-card contain-inline-size"
      >
        <div className="flex min-w-0 items-center gap-3 p-3">
          <span data-slot="result-row-mark" className="flex shrink-0 items-center">
            {mark}
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <div
                data-slot="result-row-title"
                className="min-w-0 truncate text-sm font-medium text-foreground"
              >
                {title}
              </div>
              {badge !== undefined && (
                <span className="flex max-w-full min-w-0 items-center gap-1 overflow-hidden">
                  {badge}
                </span>
              )}
            </div>
            {meta !== undefined && (
              <div
                data-slot="result-row-meta"
                className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground"
              >
                {meta}
              </div>
            )}
          </div>
          {actions !== undefined && (
            <div className="flex shrink-0 items-center gap-2">{actions}</div>
          )}
          <ExpandToggle
            expanded={expanded}
            label={toggleLabel}
            slot="result-row-toggle"
          />
        </div>
        <CollapsibleContent
          data-slot="result-row-body"
          className="min-w-0 border-t border-border p-3"
        >
          {children}
        </CollapsibleContent>
      </li>
    </Collapsible>
  );
}
