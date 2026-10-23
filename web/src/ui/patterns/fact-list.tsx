import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { useDensity } from "./density";

// See web/registry/rules/patterns.md, Fact list.

export interface Fact {
  id: string;
  term: ReactNode;
  value: ReactNode;
}

export interface FactListProps {
  facts: Fact[];
  /** `divided`: one hairline row per fact, the value at the right end; for a column of figures. */
  variant?: "grid" | "divided";
  /** `grid` only: split the facts into two lists side by side from `md`, the first half left. */
  columns?: 1 | 2;
  /** Names the list for screen readers when no heading labels it. */
  "aria-label"?: string;
}

function GridList({
  facts,
  viewing,
  label,
}: {
  facts: Fact[];
  viewing: boolean;
  label?: string;
}) {
  return (
    <dl
      data-slot="fact-list"
      aria-label={label}
      className={cn("grid grid-cols-3 gap-x-4", viewing ? "gap-y-3" : "gap-y-2")}
    >
      {facts.map((fact) => (
        <div key={fact.id} className="col-span-3 grid grid-cols-subgrid">
          <dt className="text-muted-foreground">{fact.term}</dt>
          <dd className="col-span-2 min-w-0 break-words">{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function FactList({
  facts,
  variant = "grid",
  columns = 1,
  "aria-label": label,
}: FactListProps) {
  const viewing = useDensity() === "viewing";
  if (variant === "divided") {
    return (
      <dl data-slot="fact-list" data-variant="divided" aria-label={label}>
        {facts.map((fact) => (
          <div
            key={fact.id}
            className="flex items-baseline justify-between gap-4 border-b border-border py-2"
          >
            <dt className="text-xs text-muted-foreground">{fact.term}</dt>
            <dd className="min-w-0 text-right font-mono text-xs break-words tabular-nums">
              {fact.value}
            </dd>
          </div>
        ))}
      </dl>
    );
  }
  if (columns === 2 && facts.length > 1) {
    const half = Math.ceil(facts.length / 2);
    return (
      <div
        data-slot="fact-list-columns"
        role={label === undefined ? undefined : "group"}
        aria-label={label}
        className={cn("grid gap-x-10 md:grid-cols-2", viewing ? "gap-y-3" : "gap-y-2")}
      >
        <GridList facts={facts.slice(0, half)} viewing={viewing} />
        <GridList facts={facts.slice(half)} viewing={viewing} />
      </div>
    );
  }
  return <GridList facts={facts} viewing={viewing} label={label} />;
}
