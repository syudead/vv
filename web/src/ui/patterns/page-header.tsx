import type { ReactNode, Ref } from "react";

// The skeleton owns outer spacing; see web/registry/rules/patterns.md, Page header.

export interface PageHeaderProps {
  title: ReactNode;
  /** Use level 2 for a section within a combined page. */
  headingLevel?: 1 | 2;
  count?: ReactNode;
  description?: ReactNode;
  leading?: ReactNode;
  /** At most one default primary button. */
  actions?: ReactNode;
  /** Makes h1 programmatically focusable as a fallback after the last item is removed. */
  titleRef?: Ref<HTMLHeadingElement>;
  /** Hide visually when breadcrumbs already show the same title. */
  titleHidden?: boolean;
  /** Wrap a long title in full instead of truncating it to one line, as a record's name. */
  wrapTitle?: boolean;
}

function titleClass(hidden: boolean, wrap: boolean): string {
  if (hidden) return "sr-only";
  return wrap
    ? "min-w-0 text-xl font-semibold wrap-anywhere"
    : "min-w-0 truncate text-xl font-semibold";
}

export function PageHeader({
  title,
  headingLevel = 1,
  count,
  description,
  leading,
  actions,
  titleRef,
  titleHidden = false,
  wrapTitle = false,
}: PageHeaderProps) {
  const Heading = headingLevel === 1 ? "h1" : "h2";
  return (
    <header data-slot="page-header" className="flex flex-col gap-2">
      {leading && (
        <div data-slot="page-header-leading" className="flex items-center gap-2">
          {leading}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 items-baseline gap-2">
            <Heading
              ref={titleRef}
              tabIndex={titleRef === undefined ? undefined : -1}
              className={titleClass(titleHidden, wrapTitle)}
            >
              {title}
            </Heading>
            {count !== undefined && (
              <span
                data-slot="page-header-count"
                className="shrink-0 text-xs text-muted-foreground tabular-nums"
              >
                {count}
              </span>
            )}
          </div>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && (
          <div
            data-slot="page-header-actions"
            className="flex max-w-full shrink-0 flex-wrap items-center gap-2"
          >
            {actions}
          </div>
        )}
      </div>
    </header>
  );
}
