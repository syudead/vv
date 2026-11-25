import { createContext, type ReactNode, type Ref, useContext } from "react";

// The skeleton owns outer spacing; see web/registry/rules/patterns.md, Page header.

/** ListPage's `filter` slot, drawn by the PageHeader in its `header` slot. */
export const PageHeaderFilterContext = createContext<ReactNode>(undefined);

/** Whether an optional slot was filled; `false` and `null` leave it empty. */
export function present(node: ReactNode): boolean {
  return node !== undefined && node !== null && node !== false;
}

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
  const filter = useContext(PageHeaderFilterContext);
  const hasFilter = present(filter);
  return (
    <header data-slot="page-header" className="flex flex-col gap-2">
      {leading && (
        <div data-slot="page-header-leading" className="flex items-center gap-2">
          {leading}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div
          className={
            hasFilter
              ? "flex min-w-0 flex-1 flex-col gap-1 lg:max-w-1/2 lg:flex-none"
              : "flex min-w-0 flex-1 flex-col gap-1"
          }
        >
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
        {hasFilter && (
          // Its own line under the title and actions below lg; between them from lg. The inset keeps the shared focus ring inside the scroll area.
          <div
            data-slot="page-header-filter"
            className="order-last -m-1 min-w-0 grow basis-full overflow-x-auto p-1 lg:order-none lg:flex-1 lg:basis-0"
          >
            {filter}
          </div>
        )}
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
