import type { ReactNode } from "react";

// The reading column SettingsPage and RecordPage share; see web/registry/rules/patterns.md, Record page.

export interface PageColumnBodyProps {
  /** Prefix of the data-slot names, the skeleton's own slot. */
  slot: string;
  notice?: ReactNode;
  children: ReactNode;
}

/** The optional notice, then the sections `gap-8` apart. */
export function PageColumnBody({ slot, notice, children }: PageColumnBodyProps) {
  return (
    <>
      {notice !== undefined && <div data-slot={`${slot}-notice`}>{notice}</div>}
      <div data-slot={`${slot}-body`} className="flex flex-col gap-8">
        {children}
      </div>
    </>
  );
}

export interface PageColumnProps extends PageColumnBodyProps {
  header: ReactNode;
}

/** One column at most `max-w-3xl`, centered: the header, the notice and the sections. */
export function PageColumn({ slot, header, notice, children }: PageColumnProps) {
  return (
    <div
      data-slot={slot}
      className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-3 sm:p-4"
    >
      {header}
      <PageColumnBody slot={slot} notice={notice}>
        {children}
      </PageColumnBody>
    </div>
  );
}
