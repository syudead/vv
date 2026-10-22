import { AlertCircle } from "lucide-react";
import { type ReactNode, useId } from "react";

import { formatDate, formatDateTime, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../../ui/shadcn/popover";
import { factTrigger } from "../factTrigger";

/** A panel's section: a small muted heading over its content. */
export function PanelSection({
  title,
  children,
}: {
  title: UiText;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <section aria-labelledby={id} className="flex flex-col gap-3">
      <h2
        id={id}
        className="text-xs font-medium tracking-wider text-muted-foreground uppercase"
      >
        {title}
      </h2>
      {children}
    </section>
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
          className={cn(factTrigger, "text-base text-foreground tabular-nums")}
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
