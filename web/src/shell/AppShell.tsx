import type { ReactNode } from "react";

import { SidebarInset, SidebarProvider } from "../ui/shadcn/sidebar";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";

// 640px 以上のサイドバーはいつもレールで、展開しない。開閉の選択は持たず、Ctrl/⌘+B も
// 聞かない（何も変わらないキーを奪って、ブラウザのブックマークバーの切り替えを塞がないため）。
const railOnly = () => undefined;

/**
 * AppShell は上部バーと左サイドバーを被せ、中身をその右下に置く。サイドバーは
 * shadcn/ui の Sidebar で、640px 以上は閉じた状態（レール）に固定する。639px 以下の
 * ドロワーの開閉は SidebarProvider が自分で持つ。
 */
export default function AppShell({ children }: { children: ReactNode }) {
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
      </div>
    </SidebarProvider>
  );
}
