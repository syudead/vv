import type { ReactNode } from "react";

import { present } from "./page-header";

// Organizing one record: its names, its properties and the operations on it. See web/registry/rules/patterns.md, Record editor page.

export interface RecordEditorPageProps {
  /** The router link back to the list, as a `ghost` `sm` Button with `ArrowLeft`. */
  back: ReactNode;
  /** The record's name, the page's `h1`; it wraps in full. */
  title: ReactNode;
  /** Drawn in the title's place while set: the field that renames the record, or a `Skeleton` while it loads. The `h1` stays for screen readers. */
  titleField?: ReactNode;
  /** One or two `RecordEditorFigure`s after the title. */
  figures?: ReactNode;
  /** `sm` `pill` buttons: at most one `default`, then `secondary`, then at most one `ghost-destructive` after a `Separator`. */
  actions?: ReactNode;
  /** One `Alert` under the header: a failed or stale operation, or one that made another record. */
  notice?: ReactNode;
  /** `PageSection`s for the record's lists; the first column from lg. */
  main: ReactNode;
  /** `PageSection`s for single properties and operations that take another record; the second column from lg. */
  side?: ReactNode;
  /** Replaces everything under `back`, such as when the record no longer exists. */
  status?: ReactNode;
  /** One `Shelf` of the items the record is on. */
  children?: ReactNode;
}

export function RecordEditorPage({
  back,
  title,
  titleField,
  figures,
  actions,
  notice,
  main,
  side,
  status,
  children,
}: RecordEditorPageProps) {
  const field = present(titleField);
  return (
    <div
      data-slot="record-editor-page"
      className="mx-auto flex w-full max-w-6xl flex-col gap-6 p-3 sm:p-4"
    >
      <div data-slot="record-editor-page-back" className="flex items-center gap-2">
        {back}
      </div>
      {status ?? (
        <>
          <header
            data-slot="record-editor-page-header"
            className="flex flex-wrap items-center gap-x-3 gap-y-2"
          >
            <div className="flex min-w-0 flex-1 basis-1/2 flex-wrap items-baseline gap-x-3 gap-y-1">
              <h1
                className={
                  field ? "sr-only" : "min-w-0 text-xl font-semibold wrap-anywhere"
                }
              >
                {title}
              </h1>
              {field && <div className="min-w-0 flex-1">{titleField}</div>}
              {present(figures) && (
                <div
                  data-slot="record-editor-page-figures"
                  className="flex shrink-0 items-baseline gap-x-3"
                >
                  {figures}
                </div>
              )}
            </div>
            {present(actions) && (
              <div
                data-slot="record-editor-page-actions"
                className="flex max-w-full flex-wrap items-center gap-2"
              >
                {actions}
              </div>
            )}
          </header>
          {present(notice) && <div data-slot="record-editor-page-notice">{notice}</div>}
          <div
            data-slot="record-editor-page-body"
            className={
              present(side)
                ? "flex flex-col gap-6 lg:grid lg:grid-cols-record-editor lg:items-start"
                : "flex flex-col gap-6"
            }
          >
            <div
              data-slot="record-editor-page-main"
              className="flex min-w-0 flex-col gap-6"
            >
              {main}
            </div>
            {present(side) && (
              <div
                data-slot="record-editor-page-side"
                className="flex min-w-0 flex-col gap-6"
              >
                {side}
              </div>
            )}
          </div>
          {children}
        </>
      )}
    </div>
  );
}

export interface RecordEditorFigureProps {
  /** The formatted number. */
  value: ReactNode;
  /** Its unit, such as `videos`. */
  unit: ReactNode;
}

/** A figure after a record editor page's title: the number, then its muted unit. */
export function RecordEditorFigure({ value, unit }: RecordEditorFigureProps) {
  return (
    <span data-slot="record-editor-figure" className="flex items-baseline gap-1">
      <span className="font-semibold text-foreground tabular-nums">{value}</span>
      <span className="text-sm text-muted-foreground">{unit}</span>
    </span>
  );
}
