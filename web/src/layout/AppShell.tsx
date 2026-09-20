import { type CSSProperties, type ReactNode, useCallback, useState } from "react";

import Sidebar from "./Sidebar";

const sidebarPreferenceKey = "vv.sidebar.collapsed.v1";

function readSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(sidebarPreferenceKey) === "true";
  } catch {
    return false;
  }
}

function writeSidebarCollapsed(value: boolean): void {
  try {
    localStorage.setItem(sidebarPreferenceKey, String(value));
  } catch {
    // 保存できなくても、現在の画面では切り替えを続けられる。
  }
}

export default function AppShell({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(readSidebarCollapsed);

  const toggleSidebar = useCallback(() => {
    setCollapsed((current) => {
      const next = !current;
      writeSidebarCollapsed(next);
      return next;
    });
  }, []);

  return (
    <div
      className="min-h-dvh sm:pl-[var(--sidebar-current)]"
      style={
        {
          "--sidebar-current": collapsed
            ? "var(--size-sidebar-collapsed)"
            : "var(--size-sidebar)",
        } as CSSProperties
      }
    >
      <Sidebar collapsed={collapsed} onToggle={toggleSidebar} />
      {children}
    </div>
  );
}
