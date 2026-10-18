import type { ReactNode, Ref } from "react";

// Admin table skeleton; see web/registry/rules/patterns.md, Admin table page. Supply TopBarPortal-wrapped Toolbar for the shared top bar.
// The skeleton owns spacing; replace the table slot with loading, empty or error states.

export interface AdminTablePageProps {
  toolbar?: ReactNode;
  header: ReactNode;
  band: ReactNode;
  /** Use header instead for a selection bar replacing the page heading. */
  selectionBar?: ReactNode;
  /** Measure the sticky band so virtual rows remain visible beneath it. */
  bandRef?: Ref<HTMLDivElement>;
  children: ReactNode;
}

export function AdminTablePage({
  toolbar,
  header,
  band,
  selectionBar,
  bandRef,
  children,
}: AdminTablePageProps) {
  return (
    <div
      data-slot="admin-table-page"
      className="mx-auto flex min-h-full w-full max-w-4xl flex-col gap-3 p-3 sm:p-4"
    >
      {toolbar}
      <div
        ref={bandRef}
        data-slot="admin-table-page-band"
        className="sticky top-navbar z-35 -mx-3 -mt-3 flex flex-col gap-3 bg-background px-3 pt-3 pb-1 sm:-mx-4 sm:-mt-4 sm:px-4 sm:pt-4"
      >
        {header}
        {band}
      </div>
      <div data-slot="admin-table-page-body" className="flex flex-1 flex-col gap-3">
        {children}
      </div>
      {selectionBar}
    </div>
  );
}
