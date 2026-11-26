import { Slot } from "radix-ui";
import type { ReactElement, ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Badge } from "@/ui/shadcn/badge";
import { Checkbox } from "@/ui/shadcn/checkbox";

import { KindMark } from "./KindMark";

// See web/registry/rules/components.md, RecordCard.

export interface RecordCardSelection {
  selected: boolean;
  /** Something in the list is selected: the check shows on every card. */
  selectionMode: boolean;
  /** The check's name, such as `Select travel`. */
  label: string;
  onSelectedChange: (selected: boolean) => void;
}

export interface RecordCardProps {
  /** A router link with no content; the card's lines become its content. */
  link: ReactElement;
  /** The record's name; truncated, with the full name in `title`. */
  name: string;
  /** Marks after the name, such as `TentativeMark`. */
  nameMarks?: ReactNode;
  /** The formatted count at the first line's end. */
  count: ReactNode;
  /** The count's full wording, such as `12 videos`. */
  countTitle?: string;
  /** One short phrase on the second line. */
  detail: ReactNode;
  /** Other names, as badges on the third line; the card's edge cuts what does not fit. */
  secondary?: readonly string[];
  /** The record's kind id, drawn as a `KindMark` `bar` on the start edge. */
  kind?: number;
  /** Draws the edge dashed in `input`. */
  tentative?: boolean;
  selection?: RecordCardSelection;
}

function edgeClass(selected: boolean, tentative: boolean): string {
  if (selected) return cn("border-primary bg-primary-soft", tentative && "border-dashed");
  return tentative ? "border-dashed border-input bg-card" : "border-border bg-card";
}

/** One named record as a fixed-height link card, with an optional selection check outside the link. */
export function RecordCard({
  link,
  name,
  nameMarks,
  count,
  countTitle,
  detail,
  secondary = [],
  kind,
  tentative = false,
  selection,
}: RecordCardProps) {
  const selected = selection?.selected === true;
  return (
    <div
      data-slot="record-card"
      data-selected={selected || undefined}
      data-tentative={tentative || undefined}
      className="group/record-card relative h-record-card w-full min-w-0 contain-inline-size"
    >
      <Slot.Root
        className={cn(
          "relative flex h-full flex-col gap-1 overflow-hidden rounded-md border py-3 ps-4 pe-3 text-card-foreground transition-shadow hover:shadow-card-hover",
          edgeClass(selected, tentative),
        )}
      >
        <Slot.Slottable>{link}</Slot.Slottable>
        {kind !== undefined && <KindMark kind={kind} variant="bar" />}
        <span className="flex min-w-0 items-center gap-1">
          <span title={name} className="min-w-0 truncate text-sm font-medium">
            {name}
          </span>
          {nameMarks}
          <span
            title={countTitle}
            className="ms-auto shrink-0 ps-2 text-sm font-semibold tabular-nums"
          >
            {count}
          </span>
        </span>
        <span className="truncate text-xs text-muted-foreground">{detail}</span>
        {secondary.length > 0 && (
          <span
            className={cn(
              "mt-auto flex min-w-0 gap-1 overflow-hidden",
              selection !== undefined && "pe-6",
            )}
          >
            {secondary.map((other) => (
              <Badge key={other} variant="secondary" className="font-normal">
                {other}
              </Badge>
            ))}
          </span>
        )}
      </Slot.Root>
      {selection !== undefined && (
        <span
          className={cn(
            "absolute end-3 bottom-4 z-10 flex transition-opacity duration-150",
            selection.selectionMode || selected
              ? "opacity-100"
              : "opacity-0 group-focus-within/record-card:opacity-100 group-hover/record-card:opacity-100 [@media(hover:none)]:opacity-100",
          )}
        >
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => selection.onSelectedChange(next === true)}
            aria-label={selection.label}
          />
        </span>
      )}
    </div>
  );
}
