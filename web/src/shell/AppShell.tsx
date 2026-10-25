import type { ReactNode } from "react";

import { SidebarInset, SidebarProvider } from "../ui/shadcn/sidebar";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";

// The desktop rail never expands; do not capture Ctrl/Meta+B and block the browser's bookmark shortcut.
const railOnly = () => undefined;

/**
 * Desktop stays collapsed; SidebarProvider owns only the narrow-screen drawer state. `panel` takes one
 * SidePanel after the content, which narrows the content row from lg.
 */
export default function AppShell({
  children,
  panel,
}: {
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
        <SidebarInset>{children}</SidebarInset>
        {panel}
      </div>
    </SidebarProvider>
  );
}
