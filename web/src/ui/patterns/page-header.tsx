import { createContext, type ReactNode, type Ref, useContext, useRef } from "react";

import { useScrollEdges } from "@/hooks/useScrollEdges";

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

/**
 * The filter's own row under the title and actions at every width, so no choice hides beside them. Below md it is
 * one line that scrolls sideways and fades at a side that hides choices; from md the group wraps.
 */
function PageHeaderFilter({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const edges = useScrollEdges(ref);
  return (
    // The inset keeps the shared focus ring inside the scroll area.
    <div
      ref={ref}
      data-slot="page-header-filter"
      data-scroll-start={edges.start ? "" : undefined}
      data-scroll-end={edges.end ? "" : undefined}
      className="-m-1 min-w-0 overflow-x-auto p-1 max-md:mask-scroll-edges md:overflow-x-visible"
    >
      {children}
    </div>
  );
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
        {/* With a filter the actions are a list's links and create, wider than a narrow screen
            leaves beside the title, so the title keeps its width and the actions wrap under it. */}
        <div
          className={
            hasFilter
              ? "flex min-w-0 flex-auto flex-col gap-1"
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
        {actions && (
          <div
            data-slot="page-header-actions"
            className="flex max-w-full shrink-0 flex-wrap items-center gap-2"
          >
            {actions}
          </div>
        )}
      </div>
      {hasFilter && <PageHeaderFilter>{filter}</PageHeaderFilter>}
    </header>
  );
}
