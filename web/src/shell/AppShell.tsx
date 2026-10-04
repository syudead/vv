import type { ReactNode } from "react";

import { SidebarInset, SidebarProvider } from "../ui/shadcn/sidebar";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";
import { useSidebarOpen } from "./useSidebar";

/**
 * AppShell は上部バーと左サイドバーを被せ、中身をその右下に置く。サイドバーは
 * shadcn/ui の Sidebar で、開閉の選択は useSidebarOpen が保存する。
 */
export default function AppShell({ children }: { children: ReactNode }) {
  const { open, setOpen } = useSidebarOpen();

  return (
    <SidebarProvider open={open} onOpenChange={setOpen} className="flex-col">
      <TopBar />
      <div className="flex flex-1 pt-navbar">
        <Sidebar />
        <SidebarInset>{children}</SidebarInset>
      </div>
    </SidebarProvider>
  );
}
