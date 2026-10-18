import type { ReactNode, Ref } from "react";

import { cn } from "@/lib/cn";

// This section owns card spacing, surfaces and aligned time/action columns; callers supply content and tone. See web/registry/rules/patterns.md, Sections.
//

export type CardListTone = "default" | "primary" | "destructive" | "warning";

const toneClass: Record<CardListTone, string> = {
  default: "border-border bg-muted",
  primary: "border-primary bg-primary-soft",
  destructive: "border-destructive-strong bg-destructive-soft",
  warning: "border-warning bg-warning-soft",
};

export interface CardListProps {
  label: string;
  children: ReactNode;
}

function CardListRoot({ label, children }: CardListProps) {
  return (
    <ul data-slot="card-list" aria-label={label} className="flex flex-col gap-2">
      {children}
    </ul>
  );
}

export interface CardListItemProps {
  tone?: CardListTone;
  icon: ReactNode;
  /** Truncate the title before other columns. */
  title: ReactNode;
  /** Chips never truncate. */
  badge?: ReactNode;
  /** Truncate after the title; apply truncate to the nested text element. */
  detail?: ReactNode;
  time: ReactNode;
  timeShort: ReactNode;
  actions?: ReactNode;
  ref?: Ref<HTMLLIElement>;
}

function CardListItem({
  tone = "default",
  icon,
  title,
  badge,
  detail,
  time,
  timeShort,
  actions,
  ref,
}: CardListItemProps) {
  return (
    <li
      ref={ref}
      data-slot="card-list-item"
      data-tone={tone}
      className={cn(
        "flex min-h-activity-card min-w-0 items-center gap-3 rounded-lg border px-4 py-2",
        toneClass[tone],
      )}
    >
      <span data-slot="card-list-icon" className="flex shrink-0 items-center">
        {icon}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1 lg:flex-row lg:items-center lg:gap-3">
        <div
          data-slot="card-list-title"
          className="min-w-0 truncate text-sm font-medium text-foreground lg:flex-1"
        >
          {title}
        </div>
        <div className="flex min-w-0 items-center gap-2 lg:max-w-2/3 lg:shrink-0 lg:gap-3">
          {badge !== undefined && <span className="flex shrink-0">{badge}</span>}
          <div
            data-slot="card-list-detail"
            className="flex min-w-0 flex-1 items-center gap-2 text-sm text-muted-foreground lg:justify-end"
          >
            {detail}
          </div>
          <span
            data-slot="card-list-time-short"
            className="shrink-0 text-sm whitespace-nowrap text-muted-foreground lg:hidden"
          >
            {timeShort}
          </span>
        </div>
        <span
          data-slot="card-list-time"
          className="hidden min-w-activity-time shrink-0 text-right text-sm whitespace-nowrap text-muted-foreground lg:block"
        >
          {time}
        </span>
      </div>
      <div
        data-slot="card-list-actions"
        className="flex min-w-16 shrink-0 items-center justify-end"
      >
        {actions}
      </div>
    </li>
  );
}

const CardList = Object.assign(CardListRoot, { Item: CardListItem });

export { CardList, CardListItem };
