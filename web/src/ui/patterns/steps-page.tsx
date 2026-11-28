import { Check } from "lucide-react";
import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";
import BrandHomeLink from "@/ui/BrandHomeLink";

// A page outside the shell for steps taken in a fixed order, one at a time, such as the first setup.
// The navbar-high band holds BrandHomeLink and the numbered steps strip (the current step in primary,
// finished ones marked; below sm, `Step 2 of 3` and the current name); below it one centred max-w-2xl
// column stacks the title, description, content, failure and footer. Rules: web/registry/rules/patterns.md, Steps page.

export interface StepsPageStep {
  key: string;
  /** The name shown in the strip. */
  name: ReactNode;
  /** The accessible name, which says when the step is done or current. */
  label: string;
}

export interface StepsPageProps {
  /** In the order they are taken. */
  steps: readonly StepsPageStep[];
  /** The current step's index in steps. */
  current: number;
  /** The accessible name of the strip's nav. */
  stepsLabel: string;
  /** The position shown before the current name below sm (`Step 2 of 3`). */
  progress: ReactNode;
  /** Rendered as the h1. */
  title: ReactNode;
  description?: ReactNode;
  /** Stacked from the top; the skeleton spaces them with gap-4. */
  children?: ReactNode;
  /** The failure Alert above the footer. */
  notice?: ReactNode;
  /** The footer's `Skip` at its left, a ghost sm Button. */
  skip: ReactNode;
  /** The footer's one main action at its right, a default sm Button. */
  action: ReactNode;
}

function StepMark({
  index,
  done,
  current,
}: {
  index: number;
  done: boolean;
  current: boolean;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium tabular-nums",
        current && "border-primary bg-primary text-primary-foreground",
        done && "border-primary text-primary",
        !current && !done && "border-border text-muted-foreground",
      )}
    >
      {done ? <Check className="size-3" /> : index + 1}
    </span>
  );
}

export function StepsPage({
  steps,
  current,
  stepsLabel,
  progress,
  title,
  description,
  children,
  notice,
  skip,
  action,
}: StepsPageProps) {
  const titleId = useId();
  const currentStep = steps[current];
  return (
    <div
      data-slot="steps-page"
      className="flex min-h-dvh flex-col bg-background text-foreground"
    >
      <header
        data-slot="steps-page-header"
        className="sticky top-0 z-20 flex h-navbar shrink-0 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6"
      >
        <div className="flex flex-1 basis-0 items-center">
          <BrandHomeLink />
        </div>
        <nav aria-label={stepsLabel} className="flex min-w-0 shrink justify-center">
          <ol className="hidden items-center gap-4 sm:flex">
            {steps.map((step, index) => {
              const isCurrent = index === current;
              return (
                <li
                  key={step.key}
                  aria-current={isCurrent ? "step" : undefined}
                  className="flex items-center gap-2 text-sm"
                >
                  <StepMark index={index} done={index < current} current={isCurrent} />
                  <span
                    aria-hidden="true"
                    className={cn(
                      "whitespace-nowrap",
                      isCurrent ? "font-medium text-primary" : "text-muted-foreground",
                    )}
                  >
                    {step.name}
                  </span>
                  <span className="sr-only">{step.label}</span>
                </li>
              );
            })}
          </ol>
          <p className="flex min-w-0 items-baseline gap-2 text-sm sm:hidden">
            <span className="shrink-0 text-muted-foreground">{progress}</span>
            <span className="truncate font-medium text-primary">{currentStep?.name}</span>
          </p>
        </nav>
        <div aria-hidden="true" className="hidden flex-1 basis-0 sm:block" />
      </header>
      <main
        aria-labelledby={titleId}
        data-slot="steps-page-body"
        className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 p-4 sm:p-6"
      >
        <div className="flex flex-col gap-1">
          <h1 id={titleId} className="text-xl font-semibold">
            {title}
          </h1>
          {description !== undefined && (
            <p className="text-sm text-muted-foreground">{description}</p>
          )}
        </div>
        {children !== undefined && (
          <div data-slot="steps-page-content" className="flex min-w-0 flex-col gap-4">
            {children}
          </div>
        )}
        {notice}
        <div
          data-slot="steps-page-footer"
          className="flex items-center justify-between gap-2"
        >
          {skip}
          {action}
        </div>
      </main>
    </div>
  );
}
