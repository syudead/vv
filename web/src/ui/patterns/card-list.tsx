import { ChevronRight } from "lucide-react";
import type { ReactNode, Ref } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/ui/shadcn/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";

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
    <ul
      data-slot="card-list"
      aria-label={label}
      className="group/card-list flex flex-col gap-2"
    >
      {children}
    </ul>
  );
}

/** The expand button's text for its current state: a tooltip and the name read aloud. */
export interface CardListToggleLabel {
  tooltip: string;
  name: string;
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
  /** `CardListMember` rows, shown inside the card while `expanded`; adds the expand button. */
  members?: ReactNode;
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
  /** Required with `members`. */
  toggleLabel?: CardListToggleLabel;
  ref?: Ref<HTMLLIElement>;
}

function Toggle({
  expanded,
  label,
}: {
  expanded: boolean;
  label: CardListToggleLabel | undefined;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <CollapsibleTrigger asChild>
          <Button
            data-slot="card-list-toggle"
            variant="ghost"
            size="icon-sm"
            aria-label={label?.name}
          >
            <ChevronRight
              aria-hidden="true"
              className={cn("transition-transform", expanded && "rotate-90")}
            />
          </Button>
        </CollapsibleTrigger>
      </TooltipTrigger>
      <TooltipContent>{label?.tooltip}</TooltipContent>
    </Tooltip>
  );
}

/** Keeps a card without members aligned with the expandable cards of the same list. */
function ToggleSpace() {
  return (
    <span
      aria-hidden="true"
      className="hidden size-8 shrink-0 group-has-[[data-slot=card-list-toggle]]/card-list:block"
    />
  );
}

interface FirstLineProps {
  toggle: ReactNode;
  icon: ReactNode;
  title: ReactNode;
  badge?: ReactNode;
  detail?: ReactNode;
  time: ReactNode;
  timeShort: ReactNode;
  actions?: ReactNode;
}

function FirstLine({
  toggle,
  icon,
  title,
  badge,
  detail,
  time,
  timeShort,
  actions,
}: FirstLineProps) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      {toggle}
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
    </div>
  );
}

const itemClass =
  "flex min-h-activity-card min-w-0 flex-col justify-center rounded-lg border px-4 py-2";

function CardListItem({
  tone = "default",
  members,
  expanded = false,
  onExpandedChange,
  toggleLabel,
  ref,
  ...line
}: CardListItemProps) {
  if (members === undefined)
    return (
      <li
        ref={ref}
        data-slot="card-list-item"
        data-tone={tone}
        className={cn(itemClass, toneClass[tone])}
      >
        <FirstLine toggle={<ToggleSpace />} {...line} />
      </li>
    );
  return (
    <Collapsible open={expanded} onOpenChange={onExpandedChange} asChild>
      <li
        ref={ref}
        data-slot="card-list-item"
        data-tone={tone}
        className={cn(itemClass, toneClass[tone])}
      >
        <FirstLine
          toggle={<Toggle expanded={expanded} label={toggleLabel} />}
          {...line}
        />
        <CollapsibleContent className="mt-2 border-t border-inherit pt-1">
          <div className="flex min-w-0 gap-3">
            <span aria-hidden="true" className="w-8 shrink-0" />
            <span aria-hidden="true" className="w-5 shrink-0" />
            <ul data-slot="card-list-members" className="flex min-w-0 flex-1 flex-col">
              {members}
            </ul>
          </div>
        </CollapsibleContent>
      </li>
    </Collapsible>
  );
}

export interface CardListMemberProps {
  /** A `size-4` status icon with its name. */
  icon: ReactNode;
  /** Truncated last, after the detail. */
  title: ReactNode;
  /** Truncated first; apply truncate to the nested text element. */
  detail?: ReactNode;
  actions?: ReactNode;
}

/** One row of an expanded card, lined up with the card's title and action columns. */
function CardListMember({ icon, title, detail, actions }: CardListMemberProps) {
  return (
    <li data-slot="card-list-member" className="flex min-h-8 min-w-0 items-center gap-3">
      <div className="flex min-w-0 flex-1 items-center gap-2 text-sm">
        <span className="flex shrink-0 items-center">{icon}</span>
        <span
          data-slot="card-list-member-title"
          className="min-w-0 truncate text-foreground"
        >
          {title}
        </span>
        <span
          data-slot="card-list-member-detail"
          className="flex min-w-0 flex-1 items-center gap-2 text-muted-foreground lg:justify-end"
        >
          {detail}
        </span>
      </div>
      <div className="flex min-w-16 shrink-0 items-center justify-end">{actions}</div>
    </li>
  );
}

const CardList = Object.assign(CardListRoot, {
  Item: CardListItem,
  Member: CardListMember,
});

export { CardList, CardListItem, CardListMember };
