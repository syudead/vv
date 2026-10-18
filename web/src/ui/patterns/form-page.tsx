import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { FieldLegend, FieldSet } from "@/ui/shadcn/field";

// Full-page batch editing with independent wide-screen columns; see web/registry/rules/patterns.md, Form page.

export interface FormPagePanel {
  key: string;
  legend: ReactNode;
  content: ReactNode;
}

export interface FormPageProps {
  title: ReactNode;
  detail?: ReactNode;
  actions: ReactNode;
  notice?: ReactNode;
  aside?: ReactNode;
  asideLabel?: string;
  panels: readonly FormPagePanel[];
  /** Replaces columns with a body state such as a load failure. */
  status?: ReactNode;
}

export function FormPage({
  title,
  detail,
  actions,
  notice,
  aside,
  asideLabel,
  panels,
  status,
}: FormPageProps) {
  return (
    <div
      data-slot="form-page"
      className="flex min-h-dvh flex-col bg-background text-foreground lg:h-dvh lg:min-h-0 lg:overflow-hidden"
    >
      <header
        data-slot="form-page-header"
        className="sticky top-0 z-20 flex h-navbar shrink-0 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6"
      >
        <div className="flex min-w-0 flex-1 items-baseline gap-3">
          {/* A title that names what is edited truncates; beside a detail it keeps its width. */}
          <h1
            className={cn(
              "text-lg font-semibold",
              // Clipping needs the full line height, or descenders are cut off.
              detail === undefined
                ? "min-w-0 truncate leading-normal"
                : "shrink-0 leading-none",
            )}
          >
            {title}
          </h1>
          {detail !== undefined && (
            <p className="hidden min-w-0 truncate text-sm text-muted-foreground sm:block">
              {detail}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      </header>
      <div
        data-slot="form-page-body"
        className="flex flex-1 flex-col gap-6 px-4 py-6 sm:px-6 lg:min-h-0"
      >
        {notice}
        {status ?? (
          <div className="grid flex-1 grid-cols-1 gap-6 lg:min-h-0 lg:grid-cols-form-page">
            {aside !== undefined && (
              <aside
                aria-label={asideLabel}
                className="flex min-w-0 flex-col gap-3 text-sm"
              >
                {aside}
              </aside>
            )}
            {panels.map((panel, index) => (
              <div
                key={panel.key}
                data-slot="form-page-panel"
                className={cn(
                  // Pad scrolling fields so input focus rings are not clipped.
                  "min-w-0 lg:-mx-1 lg:min-h-0 lg:overflow-y-auto lg:px-1",
                  // Without an aside, let the first panel occupy its column.
                  aside === undefined && index === 0 && "lg:col-span-2",
                )}
              >
                <FieldSet className="min-h-full min-w-0">
                  <FieldLegend>{panel.legend}</FieldLegend>
                  {panel.content}
                </FieldSet>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
