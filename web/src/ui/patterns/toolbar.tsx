import { SlidersHorizontal } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/ui/shadcn/popover";

// Toolbar layout rules: web/registry/rules/patterns.md, Sections.
// Top-bar placement never wraps; controls move into the compact popover below their inlineFrom breakpoint.
// End search placement follows filters and caps at max-w-sm; below sm it occupies a full row.

export type ToolbarBreakpoint = "md" | "lg" | "xl";

export interface ToolbarViewControl {
  id: string;
  label: string;
  /** Supply aria-label because inline controls have no visible label; do not assign an id. */
  control: ReactNode;
  compact?: ReactNode;
  /** Suppress the duplicate label when compact already includes its own heading. */
  compactLabelled?: boolean;
  inlineFrom?: ToolbarBreakpoint;
  /** Hide meaningless single-column sizing controls below sm, even in the popover. */
  hideBelowSm?: boolean;
}

export interface ToolbarProps {
  search?: ReactNode;
  children?: ReactNode;
  view?: ToolbarViewControl[];
  viewLabel?: string;
  actions?: ReactNode;
  /** Wrap topBar placement in TopBarPortal. */
  placement?: "page" | "topBar";
  searchPlacement?: "start" | "end";
  /** Let the page's search button expose the field below lg; keep it visible above lg. */
  searchHiddenBelowLg?: boolean;
  /**
   * Use horizontal overflow instead of wrapping; below md callers provide the search button and
   * overlay input.
   */
  nowrap?: boolean;
}

export function Toolbar(props: ToolbarProps) {
  return props.placement === "topBar" ? (
    <TopBarToolbar {...props} />
  ) : (
    <PageToolbar {...props} />
  );
}

function PageToolbar({
  search,
  children,
  view,
  viewLabel,
  actions,
  searchPlacement = "start",
  searchHiddenBelowLg = false,
  nowrap = false,
}: ToolbarProps) {
  const hasView = view !== undefined && view.length > 0;
  const atEnd = searchPlacement === "end";
  const searchSlot = search && (
    <div
      data-slot="toolbar-search"
      className={cn(
        nowrap
          ? "hidden md:block md:min-w-search-min md:flex-1 md:max-w-md lg:min-w-search-min-sm"
          : "w-full sm:w-auto sm:min-w-search-min-sm sm:flex-1",
        !nowrap && (atEnd ? "sm:ml-auto sm:max-w-sm" : "sm:max-w-md"),
        searchHiddenBelowLg && "hidden lg:block",
      )}
    >
      {search}
    </div>
  );
  return (
    <div
      data-slot="toolbar"
      className={cn(
        "flex items-center gap-2",
        nowrap ? "relative flex-nowrap overflow-x-auto" : "flex-wrap",
      )}
    >
      {!atEnd && searchSlot}
      {children}
      {atEnd && searchSlot}
      {(hasView || actions) && (
        <div data-slot="toolbar-end" className="ml-auto flex items-center gap-2">
          {hasView && (
            <>
              <div data-slot="toolbar-view" className="hidden items-center gap-2 lg:flex">
                {view.map((item) => (
                  <Fragment key={item.id}>{item.control}</Fragment>
                ))}
              </div>
              <Popover>
                <PopoverTrigger asChild>
                  <Button variant="outline" size="sm" className="lg:hidden">
                    <SlidersHorizontal aria-hidden="true" />
                    {viewLabel}
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" className="flex w-popover flex-col gap-4">
                  {view.map((item) => (
                    <div
                      key={item.id}
                      data-slot="toolbar-view-item"
                      className="flex flex-col items-start gap-1.5"
                    >
                      <span className="text-xs font-medium text-muted-foreground">
                        {item.label}
                      </span>
                      {item.control}
                    </div>
                  ))}
                </PopoverContent>
              </Popover>
            </>
          )}
          {actions}
        </div>
      )}
    </div>
  );
}

// Keep breakpoint class strings literal so Tailwind generates them.
const inlineClass: Record<ToolbarBreakpoint, string> = {
  md: "hidden md:block",
  lg: "hidden lg:block",
  xl: "hidden xl:block",
};
const compactClass: Record<ToolbarBreakpoint, string> = {
  md: "md:hidden",
  lg: "lg:hidden",
  xl: "xl:hidden",
};
const steps: ToolbarBreakpoint[] = ["md", "lg", "xl"];

function inlineStep(item: ToolbarViewControl): ToolbarBreakpoint {
  return item.inlineFrom ?? "lg";
}

/** Hide the compact menu only at the widest inlineFrom breakpoint, when every control is inline. */
function TopBarToolbar({ search, children, view, viewLabel, actions }: ToolbarProps) {
  const items = view ?? [];
  const widest = steps.reduce<ToolbarBreakpoint | undefined>(
    (found, step) => (items.some((item) => inlineStep(item) === step) ? step : found),
    undefined,
  );
  return (
    <div
      data-slot="toolbar"
      data-placement="top-bar"
      className="flex min-w-0 flex-1 items-center justify-center gap-1.5"
    >
      {search && (
        <div
          data-slot="toolbar-search"
          className="min-w-search-min flex-1 sm:max-w-md sm:min-w-search-min-sm"
        >
          {search}
        </div>
      )}
      {children}
      {items.map((item) => (
        <div
          key={item.id}
          data-slot="toolbar-view"
          className={inlineClass[inlineStep(item)]}
        >
          {item.control}
        </div>
      ))}
      {widest !== undefined && (
        <Popover>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={viewLabel}
              className={compactClass[widest]}
            >
              <SlidersHorizontal aria-hidden="true" />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="flex w-popover-wide flex-col gap-4">
            {items.map((item) => (
              <div
                key={item.id}
                data-slot="toolbar-view-item"
                className={cn(
                  "flex flex-col items-stretch gap-1.5",
                  compactClass[inlineStep(item)],
                  item.hideBelowSm === true && "hidden sm:flex",
                )}
              >
                {item.compactLabelled !== true && (
                  <span className="text-xs font-medium text-muted-foreground">
                    {item.label}
                  </span>
                )}
                {item.compact ?? item.control}
              </div>
            ))}
          </PopoverContent>
        </Popover>
      )}
      {actions}
    </div>
  );
}
