import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { useDensity } from "./density";

// 事実の一覧（区画）。項目名と値の組を 2 列に並べる。詳細ページの情報欄や設定の
// 読み取り専用の値に使う。規則は web/registry/rules/patterns.md の Fact list。

export interface Fact {
  /** 一覧の中で一意な鍵。 */
  id: string;
  term: ReactNode;
  value: ReactNode;
}

export interface FactListProps {
  facts: Fact[];
}

export function FactList({ facts }: FactListProps) {
  const viewing = useDensity() === "viewing";
  return (
    <dl
      data-slot="fact-list"
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
