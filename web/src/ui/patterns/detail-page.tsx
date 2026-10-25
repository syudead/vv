import type { CSSProperties, ReactNode } from "react";

import { cn } from "@/lib/cn";

import { DensityProvider } from "./density";

// Detail-page skeleton; see web/registry/rules/patterns.md, Detail page.
// On wide screens, constrain height in an lg:h-dvh flex column to scroll content and aside independently without double page scrolling.
// Inner parts use the split variant (lg outside theater mode), so theater mode keeps the stacked layout at every width.

export interface DetailPageProps {
  header: ReactNode;
  media: ReactNode;
  aside?: ReactNode;
  children: ReactNode;
  /**
   * Constrain content to the media's width and center it when media height limits make it narrower
   * than its column.
   */
  contentClassName?: string;
  /** Root CSS variables consumed by contentClassName. */
  style?: CSSProperties;
  /**
   * Stack the media, content and aside at every width, so the media spans the window. Only an
   * attribute changes, so toggling it keeps the media mounted.
   */
  theater?: boolean;
  /** One SidePanel beside the content row, under the header band. */
  panel?: ReactNode;
}

export function DetailPage({
  header,
  media,
  aside,
  children,
  contentClassName,
  style,
  theater = false,
  panel,
}: DetailPageProps) {
  return (
    <DensityProvider density="viewing">
      <div
        data-slot="detail-page"
        data-theater={theater ? "" : undefined}
        style={style}
        className="flex flex-col bg-background pb-16 lg:min-h-0 lg:flex-1 lg:pb-0"
      >
        <div
          data-slot="detail-page-header"
          className="sticky top-0 z-40 flex h-navbar shrink-0 items-center gap-1 border-b border-border bg-background/90 px-2 backdrop-blur-md sm:px-3"
        >
          {header}
        </div>
        <div className="flex w-full split:min-h-0 split:flex-1">
          <div className="flex min-w-0 flex-1 flex-col gap-5 split:min-h-0 split:flex-row split:gap-6 split:px-6">
            {/*
             * Expand scrolling edges with padding to preserve focus rings; establish a relative
             * container so absolute accessibility elements cannot enlarge the page.
             */}
            <div
              data-slot="detail-page-main"
              className="flex min-w-0 flex-col gap-5 split:relative split:-mx-1 split:min-h-0 split:flex-1 split:scrollbar-none split:overflow-y-auto split:overscroll-contain split:px-1 split:pt-6"
            >
              <div
                data-slot="detail-page-content"
                className={cn(
                  "flex flex-col gap-6 split:mx-auto split:w-full",
                  contentClassName,
                )}
              >
                {media}
                {/* Room under the last child, so it is not read against the window's edge. */}
                <div className="flex flex-col gap-3 px-4 pb-10 sm:px-6 split:px-0 split:pb-16">
                  {children}
                </div>
              </div>
            </div>
            {/*
             * Extend the wide-screen aside to the viewport edge for its scrollbar; inner lists own
             * right padding.
             */}
            {aside && (
              <aside
                data-slot="detail-page-aside"
                className="min-w-0 border-t border-border px-4 pt-5 sm:px-6 split:flex split:min-h-0 split:w-detail-aside split:shrink-0 split:flex-col split:-mr-6 split:border-t-0 split:border-l split:pt-6 split:pr-0 split:pl-5 split:xl:w-detail-aside-wide"
              >
                {aside}
              </aside>
            )}
          </div>
          {panel}
        </div>
      </div>
    </DensityProvider>
  );
}
