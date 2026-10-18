import { Fragment, type ReactNode, useEffect, useId, useRef } from "react";

import { Button } from "@/ui/shadcn/button";
import { Separator } from "@/ui/shadcn/separator";
import { Skeleton } from "@/ui/shadcn/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";

// Jump navigation for ListPage aside; see web/registry/rules/patterns.md, Sections.
// On narrow screens, use a scrolling chip row without the action; reveal the selected item when loaded.

export interface JumpListItem {
  value: string;
  label: ReactNode;
}

export interface JumpListProps {
  title: string;
  groups: readonly (readonly JumpListItem[])[];
  value: string | null;
  /** Selecting the active item again returns null. */
  onValueChange: (value: string | null) => void;
  action?: ReactNode;
  loading?: boolean;
  /** Hide the entire narrow-screen strip when empty. */
  emptyText?: ReactNode;
  failed?: { message: ReactNode; retryLabel: ReactNode; onRetry: () => void };
}

export function JumpList({
  title,
  groups,
  value,
  onValueChange,
  action,
  loading = false,
  emptyText,
  failed,
}: JumpListProps) {
  const filled = groups.filter((group) => group.length > 0);
  const strip = useRef<HTMLDivElement>(null);
  const headingId = useId();
  const items = filled.flat();
  const itemsKey = items.map((item) => item.value).join(" ");

  // Scroll only the narrow horizontal strip, never the wide vertical aside.
  useEffect(() => {
    const root = strip.current;
    if (root === null || value === null) return;
    const pressed = Array.from(
      root.querySelectorAll<HTMLElement>('[data-slot="toggle-group-item"]'),
    ).find((element) => element.dataset.value === value);
    if (pressed === undefined) return;
    root.scrollLeft = Math.max(
      0,
      pressed.offsetLeft - (root.clientWidth - pressed.offsetWidth) / 2,
    );
  }, [value, itemsKey]);

  let content: ReactNode;
  // Hide the entire section below lg when the strip has no items.
  let narrowHidden = false;
  if (failed !== undefined) {
    content = (
      <div className="flex items-center gap-2 lg:flex-col lg:items-start lg:gap-1">
        <p className="text-xs text-muted-foreground">{failed.message}</p>
        <Button variant="ghost" size="sm" onClick={failed.onRetry}>
          {failed.retryLabel}
        </Button>
      </div>
    );
  } else if (loading) {
    content = (
      <div className="flex gap-2 lg:flex-col" aria-hidden="true">
        {[0, 1, 2, 3].map((index) => (
          <Skeleton key={index} className="h-8 w-16 lg:w-full" />
        ))}
      </div>
    );
  } else if (items.length === 0) {
    narrowHidden = true;
    content = <p className="text-xs text-muted-foreground">{emptyText}</p>;
  } else {
    content = (
      <ToggleGroup
        ref={strip}
        type="single"
        size="sm"
        aria-label={title}
        value={value ?? ""}
        onValueChange={(next) => onValueChange(next === "" ? null : next)}
        className="w-full gap-2 overflow-x-auto lg:flex-col lg:items-stretch lg:gap-0.5 lg:overflow-visible"
      >
        {filled.map((group, index) => (
          <Fragment key={group[0]?.value ?? index}>
            {index > 0 && (
              <>
                <Separator orientation="vertical" className="h-6 lg:hidden" />
                <Separator className="my-1 hidden lg:block" />
              </>
            )}
            {group.map((item) => (
              <ToggleGroupItem
                key={item.value}
                value={item.value}
                data-value={item.value}
                className="flex-none rounded-md border border-input px-3 font-normal text-muted-foreground aria-checked:border-primary-active lg:w-full lg:justify-start lg:border-transparent lg:px-2 lg:aria-checked:border-transparent"
              >
                {item.label}
              </ToggleGroupItem>
            ))}
          </Fragment>
        ))}
      </ToggleGroup>
    );
  }

  return (
    <section
      data-slot="jump-list"
      aria-labelledby={headingId}
      className={narrowHidden ? "hidden flex-col gap-2 lg:flex" : "flex flex-col gap-2"}
    >
      <h2
        id={headingId}
        className="sr-only text-sm font-semibold text-foreground lg:not-sr-only"
      >
        {title}
      </h2>
      {content}
      {action !== undefined && (
        <div className="hidden flex-col gap-1 lg:flex">
          <Separator className="my-1" />
          {action}
        </div>
      )}
    </section>
  );
}
