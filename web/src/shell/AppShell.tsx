import type { ReactNode } from "react";

import { SidebarInset, SidebarProvider } from "../ui/sidebar";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";
import { useSidebarPreference } from "./useSidebarPreference";

/**
 * AppShell は上部バーと左サイドバーを被せ、中身をその右下に置く。サイドバーは
 * ui/sidebar（shadcn/ui の Sidebar）で、上部バーの下から始まる。
 */
export default function AppShell({ children }: { children: ReactNode }) {
  const preference = useSidebarPreference();

  return (
    <SidebarProvider
      open={preference.open}
      onOpenChange={preference.setOpen}
      className="min-h-dvh bg-background"
    >
      <TopBar />
      <Sidebar />
      <SidebarInset className="min-h-dvh pt-navbar">{children}</SidebarInset>
    </SidebarProvider>
  );
}
