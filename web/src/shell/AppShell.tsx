import type { ReactNode } from "react";

import { cn } from "../lib/cn";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";
import { useSidebar } from "./useSidebar";

/** AppShell は上部バーと左サイドバーを被せ、中身をその右下に置く。 */
export default function AppShell({ children }: { children: ReactNode }) {
  const sidebar = useSidebar();

  return (
    <div className="min-h-dvh bg-bg">
      <TopBar onMenu={sidebar.toggle} />
      <Sidebar mode={sidebar.mode} open={sidebar.open} onClose={sidebar.close} />
      <main
        className={cn(
          "min-h-dvh pt-navbar transition-[padding] duration-200 ease-out-quart",
          sidebar.mode === "expanded" && "pl-sidebar",
          sidebar.mode === "rail" && "pl-sidebar-rail",
        )}
      >
        {children}
      </main>
    </div>
  );
}
