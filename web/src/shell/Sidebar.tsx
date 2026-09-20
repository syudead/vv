import { NavLink } from "react-router";

import { cn } from "../lib/cn";
import { useToast } from "../ui/Toast";
import { navEntries, type NavEntry } from "./navigation";
import type { SidebarMode } from "./useSidebar";

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
        ? "h-16 w-16 flex-col justify-center gap-1.5 px-1 text-[10px]"
        : "h-10 gap-4 px-3 text-sm",
      active
        ? "bg-surface-hover font-medium text-fg"
        : "text-fg-muted hover:bg-surface-hover hover:text-fg",
      "[&>svg]:size-5 [&>svg]:shrink-0",
    );

  if (entry.to === undefined) {
    return (
      <button
        type="button"
        onClick={() => toast(`「${entry.label}」は準備中です`)}
        className={cn(className(false), "w-full")}
      >
        <Icon strokeWidth={1.75} />
        <span className="truncate">{entry.label}</span>
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
          <span className="truncate">{entry.label}</span>
        </>
      )}
    </NavLink>
  );
}

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
        className={cn(
          "fixed top-topbar bottom-0 left-0 z-40 flex flex-col bg-bg transition-transform duration-200 ease-out-quart",
          mode === "expanded" && "w-sidebar",
          mode === "rail" && "w-sidebar-rail",
          drawer && "w-sidebar border-r border-border",
          drawer && !open && "-translate-x-full",
        )}
      >
        <nav
          className={cn(
            "flex flex-col gap-1 overflow-y-auto",
            mode === "rail" ? "items-center px-1 py-2" : "px-3 py-3",
          )}
        >
          {navEntries.map((entry) => (
            <Entry key={entry.id} entry={entry} mode={mode} onNavigate={onClose} />
          ))}
        </nav>
        {mode !== "rail" && (
          <div className="mt-auto px-6 py-4 text-xs text-fg-subtle">
            vv — メディアライブラリ
          </div>
        )}
      </aside>
    </>
  );
}
