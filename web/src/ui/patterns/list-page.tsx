import type { ReactNode } from "react";

// List-page skeleton; pass a TopBarPortal-wrapped Toolbar for shared top-bar placement. See web/registry/rules/patterns.md, List page.
// The skeleton owns spacing; screens fill slots, including loading, empty and error body states.

export interface ListPageProps {
  header: ReactNode;
  toolbar?: ReactNode;
  /** header shares one row from lg upward; stacked is always vertical. */
  toolbarRow?: "stacked" | "header";
  band?: ReactNode;
  /** Sticky right aside from lg; between band and body below lg. Omit for a full-width body. */
  aside?: ReactNode;
  selectionBar?: ReactNode;
  children: ReactNode;
}

export function ListPage({
  header,
  toolbar,
  toolbarRow = "stacked",
  band,
  aside,
  selectionBar,
  children,
}: ListPageProps) {
  const body = (
    <div data-slot="list-page-body" className="flex min-w-0 flex-1 flex-col gap-3">
      {children}
    </div>
  );
  return (
    <div data-slot="list-page" className="flex min-h-full flex-col gap-3 p-3 sm:p-4">
      {toolbarRow === "header" ? (
        <div
          data-slot="list-page-header-row"
          className="flex flex-col gap-3 lg:flex-row lg:items-center lg:gap-4"
        >
          <div className="min-w-0 lg:shrink-0">{header}</div>
          {toolbar !== undefined && <div className="min-w-0 lg:flex-1">{toolbar}</div>}
        </div>
      ) : (
        <>
          {header}
          {toolbar}
        </>
      )}
      {band}
      {aside !== undefined ? (
        <div className="flex flex-1 flex-col gap-3 lg:grid lg:grid-cols-list-aside lg:items-start lg:gap-6">
          <aside
            data-slot="list-page-aside"
            className="min-w-0 lg:sticky lg:top-navbar lg:order-last lg:max-h-list-aside-max lg:overflow-y-auto"
          >
            {aside}
          </aside>
          {body}
        </div>
      ) : (
        body
      )}
      {selectionBar}
    </div>
  );
}
