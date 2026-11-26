import type { ReactNode } from "react";

import { PageHeaderFilterContext, present } from "./page-header";

// List-page skeleton; pass a TopBarPortal-wrapped Toolbar for shared top-bar placement. See web/registry/rules/patterns.md, List page.
// The skeleton owns spacing; screens fill slots, including loading, empty and error body states.

export interface ListPageProps {
  header: ReactNode;
  toolbar?: ReactNode;
  /** header shares one row from lg upward; stacked is always vertical. */
  toolbarRow?: "stacked" | "header";
  /** The list's main filter, drawn by the header's PageHeader on its own row under the title and the actions. */
  filter?: ReactNode;
  band?: ReactNode;
  /** One TallyStrip between the header and the body. */
  summary?: ReactNode;
  /** One FloatingNotice, stuck under the top bar over the body without moving it. */
  notice?: ReactNode;
  /** Sticky right aside from lg; between band and body below lg. Omit for a full-width body. */
  aside?: ReactNode;
  selectionBar?: ReactNode;
  children: ReactNode;
}

function HeaderRow({
  header,
  toolbar,
  toolbarRow,
}: Pick<ListPageProps, "header" | "toolbar" | "toolbarRow">) {
  if (toolbarRow !== "header")
    return (
      <>
        {header}
        {toolbar}
      </>
    );
  return (
    <div
      data-slot="list-page-header-row"
      className="flex flex-col gap-3 lg:flex-row lg:items-center lg:gap-4"
    >
      <div className="min-w-0 lg:shrink-0">{header}</div>
      {toolbar !== undefined && <div className="min-w-0 lg:flex-1">{toolbar}</div>}
    </div>
  );
}

export function ListPage({
  header,
  toolbar,
  toolbarRow = "stacked",
  filter,
  band,
  summary,
  notice,
  aside,
  selectionBar,
  children,
}: ListPageProps) {
  const body = (
    <div data-slot="list-page-body" className="flex min-w-0 flex-1 flex-col gap-3">
      {present(notice) && (
        // No height, and -mb-3 cancels the gap, so the notice floats over the first row.
        <div
          data-slot="list-page-notice"
          className="sticky top-navbar z-20 -mb-3 flex h-0 justify-center"
        >
          <div className="mt-2 max-w-full">{notice}</div>
        </div>
      )}
      {children}
    </div>
  );
  return (
    <div data-slot="list-page" className="flex min-h-full flex-col gap-3 p-3 sm:p-4">
      <PageHeaderFilterContext value={filter}>
        <HeaderRow header={header} toolbar={toolbar} toolbarRow={toolbarRow} />
      </PageHeaderFilterContext>
      {band}
      {present(summary) && <div data-slot="list-page-summary">{summary}</div>}
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
