import { Settings } from "lucide-react";
import { NavLink } from "react-router";

import { cn } from "../lib/cn";
import { useToast } from "../ui/Toast";
import { navEntries, type NavEntry } from "./navigation";
import type { SidebarMode } from "./useSidebar";

const settingsEntry: NavEntry = {
  id: "settings",
  label: "設定",
  icon: Settings,
};

function Entry({
  entry,
  mode,
  onNavigate,
}: {
  entry: NavEntry;
  mode: SidebarMode;
  onNavigate: () => void;
}) {
  const toast = useToast();
  const rail = mode === "rail";
  const Icon = entry.icon;

  const className = (active: boolean) =>
    cn(
      "flex items-center rounded-md transition-colors duration-150 select-none",
      rail
        ? "h-14 w-14 flex-col justify-center gap-1 px-0.5 text-[10px] leading-none"
        : "h-9 gap-3 px-3 text-sm",
      active
        ? "bg-active-wash font-medium text-fg"
        : "text-fg-muted hover:bg-hover-wash hover:text-fg",
      "[&>svg]:size-[18px] [&>svg]:shrink-0",
    );

  if (entry.to === undefined) {
    return (
      <button
        type="button"
        onClick={() => toast(`「${entry.label}」は準備中です`)}
        className={cn(className(false), "w-full")}
      >
        <Icon strokeWidth={1.75} />
        <span className="max-w-full truncate">{entry.label}</span>
      </button>
    );
  }

  return (
    <NavLink
      to={entry.to}
      end
      onClick={onNavigate}
      className={({ isActive }) => className(isActive)}
    >
      {({ isActive }) => (
        <>
          <Icon strokeWidth={isActive ? 2.25 : 1.75} />
          <span className="max-w-full truncate">{entry.label}</span>
        </>
      )}
    </NavLink>
  );
}

/** Sidebar は左のナビ。展開・レール（アイコンのみ）・ドロワーの 3 態。 */
export default function Sidebar({
  mode,
  open,
  onClose,
}: {
  mode: SidebarMode;
  open: boolean;
  onClose: () => void;
}) {
  const drawer = mode === "drawer";

  return (
    <>
      {drawer && open && (
        <button
          type="button"
          aria-label="メニューを閉じる"
          onClick={onClose}
          className="fixed inset-0 z-30 bg-overlay animate-fade-in"
        />
      )}
      <aside
        aria-label="メインナビゲーション"
        aria-hidden={drawer && !open ? true : undefined}
        inert={drawer && !open ? true : undefined}
        className={cn(
          "fixed top-navbar bottom-0 left-0 z-40 flex flex-col border-r border-border bg-bg transition-transform duration-200 ease-out-quart",
          mode === "expanded" && "w-sidebar",
          mode === "rail" && "w-sidebar-rail",
          drawer && "w-sidebar",
          drawer && !open && "-translate-x-full",
        )}
      >
        <nav
          className={cn(
            "flex min-h-0 flex-1 flex-col gap-0.5 overflow-x-hidden overflow-y-auto",
            mode === "rail" ? "items-center px-1.5 py-2" : "px-2.5 py-3",
          )}
        >
          {navEntries.map((entry) => (
            <Entry key={entry.id} entry={entry} mode={mode} onNavigate={onClose} />
          ))}
        </nav>
        <nav
          aria-label="設定"
          className={cn(
            "shrink-0 border-t border-border",
            mode === "rail" ? "px-1.5 py-2" : "px-2.5 py-3",
          )}
        >
          <Entry entry={settingsEntry} mode={mode} onNavigate={onClose} />
        </nav>
      </aside>
    </>
  );
}
