import type { ReactNode } from "react";
import { useId } from "react";
import { Slot } from "radix-ui";

import { cn } from "@/lib/cn";

import { PageColumn, PageColumnBody } from "./page-column";

// Use CSS to switch list/page visibility at sm; avoid useIsMobile. See web/registry/rules/patterns.md, Settings page.

export interface SettingsPageProps {
  /** With nav, leading should contain only the back-to-index link. */
  header: ReactNode;
  children: ReactNode;
  nav?: ReactNode;
  /** Below sm, choose index-only or page-only; from sm, show both. Ignored without nav. */
  view?: "list" | "page";
  notice?: ReactNode;
}

export function SettingsPage({
  header,
  children,
  nav,
  view = "page",
  notice,
}: SettingsPageProps) {
  if (nav === undefined) {
    return (
      <PageColumn slot="settings-page" header={header} notice={notice}>
        {children}
      </PageColumn>
    );
  }
  const listView = view === "list";
  return (
    <div
      data-slot="settings-page"
      data-view={view}
      className="mx-auto w-full max-w-4xl p-3 sm:grid sm:grid-cols-settings-page sm:items-start sm:gap-6 sm:p-4"
    >
      <div
        data-slot="settings-page-nav"
        className={cn(
          "min-w-0 sm:sticky sm:top-navbar sm:max-h-settings-nav-max sm:overflow-y-auto",
          !listView && "hidden sm:block",
        )}
      >
        {nav}
      </div>
      <div
        data-slot="settings-page-main"
        className={cn(
          "min-w-0 flex-col gap-6 sm:flex sm:**:data-[slot=page-header-leading]:hidden",
          listView ? "hidden" : "flex",
        )}
      >
        {header}
        <PageColumnBody slot="settings-page" notice={notice}>
          {children}
        </PageColumnBody>
      </div>
    </div>
  );
}

export interface SettingsNavProps {
  label: string;
  children: ReactNode;
}

export function SettingsNav({ label, children }: SettingsNavProps) {
  const headingId = useId();
  return (
    <nav
      data-slot="settings-nav"
      aria-labelledby={headingId}
      className="flex flex-col gap-6 sm:gap-2"
    >
      <h2
        id={headingId}
        className="text-xl font-semibold sm:px-2 sm:text-sm sm:text-muted-foreground"
      >
        {label}
      </h2>
      {/* Align narrow-screen text with the heading while extending only the hover fill into padding. */}
      <ul className="-mx-2 flex flex-col gap-1 sm:mx-0">{children}</ul>
    </nav>
  );
}

export interface SettingsNavItemProps {
  current?: boolean;
  children: ReactNode;
}

export function SettingsNavItem({ current = false, children }: SettingsNavItemProps) {
  return (
    <li>
      <Slot.Root
        data-slot="settings-nav-item"
        data-current={current || undefined}
        aria-current={current ? "page" : undefined}
        className="flex h-8 w-full items-center rounded-md px-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground sm:data-current:bg-secondary sm:data-current:font-medium sm:data-current:text-foreground"
      >
        {children}
      </Slot.Root>
    </li>
  );
}
