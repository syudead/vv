import type { ReactNode } from "react";

import { SidebarInset, SidebarProvider } from "../ui/shadcn/sidebar";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";

// The desktop rail never expands; do not capture Ctrl/Meta+B and block the browser's bookmark shortcut.
const railOnly = () => undefined;

/**
 * Desktop stays collapsed; SidebarProvider owns only the narrow-screen drawer state. `notice` is one
 * Alert at the top of the content column, above the page, with the page's side padding. `panel` takes one
 * SidePanel after the content, which narrows the content row from lg.
 */
export default function AppShell({
  notice,
  children,
  panel,
}: {
  notice?: ReactNode;
  children: ReactNode;
  panel?: ReactNode;
}) {
  return (
    <SidebarProvider
      open={false}
      onOpenChange={railOnly}
      keyboardShortcut={false}
      className="flex-col"
    >
      <TopBar />
      <div className="flex flex-1 pt-navbar">
        <Sidebar />
        <SidebarInset>
          {notice !== undefined && notice !== null && (
            <div data-slot="app-shell-notice" className="px-3 pt-3 sm:px-4 sm:pt-4">
              {notice}
            </div>
          )}
          {children}
        </SidebarInset>
        {panel}
      </div>
    </SidebarProvider>
  );
}
