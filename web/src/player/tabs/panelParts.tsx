import { AlertCircle } from "lucide-react";
import { type ReactNode, useId } from "react";

import {
  formatDate,
  formatDateTime,
  formatPreciseTime,
  t,
  type UiText,
} from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../../ui/shadcn/popover";
import { factTrigger } from "../factTrigger";

const labelClass = "text-2xs font-medium tracking-wider text-muted-foreground uppercase";

/** A panel's section: a small muted heading over its content, `end` at the heading line's right. */
export function PanelSection({
  title,
  end,
  children,
}: {
  title: UiText;
  end?: ReactNode;
  children: ReactNode;
}) {
  const id = useId();
  const heading = (
    <h2 id={id} className={labelClass}>
      {title}
    </h2>
  );
  return (
    <section aria-labelledby={id} className="flex flex-col gap-3">
      {end === undefined ? (
        heading
      ) : (
        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          {heading}
          {end}
        </div>
      )}
      {children}
    </section>
  );
}

/**
 * A panel's two columns from `md`: what is read at the left, the figures or the one action in a
 * `facts-column` wide column at the right. Below `md` the right column follows, full width.
 */
export function PanelColumns({ main, side }: { main?: ReactNode; side: ReactNode }) {
  return (
    <div className="flex flex-col items-start gap-6 md:flex-row md:gap-12">
      {main !== undefined && (
        <div className="flex w-full min-w-0 flex-1 flex-col gap-6">{main}</div>
      )}
      <div className="flex w-full shrink-0 flex-col gap-6 md:w-facts-column">{side}</div>
    </div>
  );
}

/** The one failure line at the end of a panel. */
export function FailureLine({ failure }: { failure: UiText | null }) {
  if (failure === null) return null;
  return (
    <p role="alert" className="flex items-center gap-2 text-sm text-destructive">
      <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
      {failure}
    </p>
  );
}

/** A read that failed: the reason, then `Retry`. */
export function ReadFailure({ text, onRetry }: { text: UiText; onRetry: () => void }) {
  return (
    <p
      role="alert"
      className="flex flex-wrap items-center gap-2 text-sm text-destructive"
    >
      {text}
      <Button variant="link" size="sm" className="h-auto px-0" onClick={onRetry}>
        {t.common.retry}
      </Button>
    </p>
  );
}

/**
 * The full date and the time with seconds (and milliseconds when the value has them); the two
 * halves wrap as units in a narrow column.
 */
export function DateTimeValue({ value }: { value: string }) {
  return (
    <time dateTime={value} className="text-foreground">
      <span className="whitespace-nowrap">{formatDate(value)}</span>{" "}
      <span className="whitespace-nowrap">{formatPreciseTime(value)}</span>
    </time>
  );
}

/** The date alone; the time is in `title` and in the popover the date opens. */
export function DateValue({ label, value }: { label: UiText; value: string }) {
  const date = formatDate(value);
  const dateTime = formatDateTime(value);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          // Set the accessible name explicitly so child whitespace cannot change it.
          aria-label={`${label} ${date}`}
          title={t.player.facts.dateDetail(label, dateTime)}
          className={cn(factTrigger, "font-mono text-xs text-foreground tabular-nums")}
        >
          {date}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side="bottom"
        align="start"
        className="block w-auto px-3 py-2 text-sm whitespace-nowrap"
        // With no focusable content, opening the popover leaves focus on its trigger.
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <span>{label}</span>{" "}
        <span className="text-muted-foreground tabular-nums">{dateTime}</span>
      </PopoverContent>
    </Popover>
  );
}
