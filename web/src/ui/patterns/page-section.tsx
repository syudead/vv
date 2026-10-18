import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";

import { useDensity } from "./density";

// This section owns direct-child separators and density-dependent padding; see web/registry/rules/patterns.md, Page section.

export interface PageSectionProps {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
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
