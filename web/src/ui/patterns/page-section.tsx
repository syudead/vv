import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";

import { useDensity } from "./density";

// 節（区画）。題・説明・操作の見出しと、カードの面に並ぶ中身を持つ。中身の直下の子は
// 1 行ずつ区切り線で分け、行の上下の余白を節が付ける（管理の画面は p-3、見る画面は p-4）。
// 規則は web/registry/rules/patterns.md の Page section。

export interface PageSectionProps {
  /** 節の題。h2 になる。 */
  title: ReactNode;
  /** 題の下の説明。 */
  description?: ReactNode;
  /** 節の見出しの右に置く操作（outline か ghost の sm）。 */
  actions?: ReactNode;
  /** 行（FormRow、FactList、文）。直下の子ごとに区切る。 */
  children: ReactNode;
}

export function PageSection({ title, description, actions, children }: PageSectionProps) {
  const titleId = useId();
  const viewing = useDensity() === "viewing";
  return (
    <section
      data-slot="page-section"
      aria-labelledby={titleId}
      className="flex flex-col gap-3"
    >
      <div className="flex items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h2 id={titleId} className="text-lg font-semibold">
            {title}
          </h2>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      <div
        data-slot="page-section-body"
        className={cn(
          "flex flex-col divide-y divide-border rounded-md border border-border bg-card text-card-foreground",
          viewing ? "text-base *:p-4" : "text-sm *:p-3",
        )}
      >
        {children}
      </div>
    </section>
  );
}
