import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Skeleton } from "@/ui/shadcn/skeleton";
import { Toggle } from "@/ui/shadcn/toggle";

// See web/registry/rules/components.md, TallyStrip.

export interface Tally {
  id: string;
  /** The formatted count; leave it out while it loads to draw a `Skeleton`. */
  count?: ReactNode;
  /** The count is zero: the tally is muted, drops its trailing word and cannot be pressed unless pressed. */
  empty?: boolean;
  label: ReactNode;
  /** One short line under the label, from `md`. */
  hint?: ReactNode;
  /** The trailing word from `lg`, such as `Review`; replaced by the strip's `clearLabel` while pressed. */
  action?: ReactNode;
  pressed: boolean;
  onPressedChange: (pressed: boolean) => void;
}

export interface TallyStripProps {
  /** Names the strip for screen readers. */
  label: string;
  /** At most three. */
  tallies: readonly Tally[];
  /** The trailing word of a pressed tally, such as `Clear`. */
  clearLabel: ReactNode;
}

/** Up to three counts in equal columns, each toggling the filter it counts. */
export function TallyStrip({ label, tallies, clearLabel }: TallyStripProps) {
  return (
    <div
      role="group"
      aria-label={label}
      data-slot="tally-strip"
      className="grid auto-cols-fr grid-flow-col gap-3"
    >
      {tallies.map((tally) => (
        <TallyToggle key={tally.id} tally={tally} clearLabel={clearLabel} />
      ))}
    </div>
  );
}

function TallyToggle({ tally, clearLabel }: { tally: Tally; clearLabel: ReactNode }) {
  const quiet = tally.empty === true && !tally.pressed;
  const trailing = tally.pressed ? clearLabel : quiet ? undefined : tally.action;
  return (
    <Toggle
      data-slot="tally"
      pressed={tally.pressed}
      onPressedChange={tally.onPressedChange}
      disabled={quiet}
      className={cn(
        "h-auto min-w-0 flex-col items-start justify-start gap-0.5 rounded-md border border-border bg-card p-3 text-left whitespace-normal text-card-foreground disabled:opacity-100 data-[state=on]:border-primary data-[state=on]:text-foreground aria-pressed:border-primary aria-pressed:text-foreground sm:flex-row sm:items-center sm:gap-3",
        quiet && "text-muted-foreground",
      )}
    >
      <span className="text-lg font-semibold tabular-nums">
        {tally.count ?? (
          <Skeleton asChild className="my-1 inline-block h-5 w-8 align-middle">
            <span />
          </Skeleton>
        )}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm font-medium">{tally.label}</span>
        {tally.hint !== undefined && (
          <span className="hidden truncate text-xs font-normal text-muted-foreground md:block">
            {tally.hint}
          </span>
        )}
      </span>
      {trailing !== undefined && (
        <span className="hidden shrink-0 text-sm text-primary lg:inline">{trailing}</span>
      )}
    </Toggle>
  );
}
