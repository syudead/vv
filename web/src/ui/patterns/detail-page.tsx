import type { CSSProperties, ReactNode } from "react";

import { cn } from "@/lib/cn";

import { DensityProvider } from "./density";

// Detail-page skeleton; see web/registry/rules/patterns.md, Detail page.
// On wide screens, constrain height in an lg:h-dvh flex column to scroll content and aside independently without double page scrolling.

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
}

export function DetailPage({
  header,
  media,
  aside,
  children,
  contentClassName,
  style,
}: DetailPageProps) {
  return (
    <DensityProvider density="viewing">
      <div
        data-slot="detail-page"
        style={style}
        className="flex flex-col bg-background pb-16 lg:min-h-0 lg:flex-1 lg:pb-0"
      >
        <div
          data-slot="detail-page-header"
          className="sticky top-0 z-40 flex h-navbar shrink-0 items-center gap-1 border-b border-border bg-background/90 px-2 backdrop-blur-md sm:px-3"
        >
          {header}
        </div>
        <div className="flex w-full flex-col gap-5 lg:min-h-0 lg:flex-1 lg:flex-row lg:gap-6 lg:px-6">
          {/*
           * Expand scrolling edges with padding to preserve focus rings; establish a relative
           * container so absolute accessibility elements cannot enlarge the page.
           */}
          <div
            data-slot="detail-page-main"
            className="flex min-w-0 flex-col gap-5 lg:relative lg:-mx-1 lg:min-h-0 lg:flex-1 lg:scrollbar-none lg:overflow-y-auto lg:overscroll-contain lg:px-1 lg:pt-6"
          >
            <div
              data-slot="detail-page-content"
              className={cn("flex flex-col gap-4 lg:mx-auto lg:w-full", contentClassName)}
            >
              {media}
              {/* Room under the last child, so it is not read against the window's edge. */}
              <div className="flex flex-col gap-3 px-4 pb-10 sm:px-6 lg:px-0 lg:pb-16">
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
              className="min-w-0 border-t border-border px-4 pt-5 sm:px-6 lg:flex lg:min-h-0 lg:w-detail-aside lg:shrink-0 lg:flex-col lg:-mr-6 lg:border-t-0 lg:border-l lg:pt-6 lg:pr-0 lg:pl-5 xl:w-detail-aside-wide"
            >
              {aside}
            </aside>
          )}
        </div>
      </div>
    </DensityProvider>
  );
}
