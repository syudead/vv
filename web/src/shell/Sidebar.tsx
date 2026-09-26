import { LogIn, LogOut, Settings } from "lucide-react";
import { useState } from "react";
import { NavLink, useLocation } from "react-router";

import { logout } from "../api/auth";
import { useAudience } from "../auth/audience";
import { currentPath, loginPath, reloadPage } from "../auth/pageNavigation";
import { cn } from "../lib/cn";
import { useToast } from "../ui/Toast";
import { navEntries, type NavEntry } from "./navigation";
import type { SidebarMode } from "./useSidebar";

const settingsEntry: NavEntry = {
  id: "settings",
  label: "設定",
  icon: Settings,
  to: "/settings",
};

function entryClassName(mode: SidebarMode, active: boolean): string {
  const rail = mode === "rail";
  return cn(
    "flex items-center rounded-md transition-colors duration-150 select-none",
    rail
      ? "h-14 w-14 flex-col justify-center gap-1 px-0.5 text-[10px] leading-none"
      : "h-9 gap-3 px-3 text-sm",
    active
      ? "bg-active-wash font-medium text-fg"
      : "text-fg-muted hover:bg-hover-wash hover:text-fg",
    "[&>svg]:size-[18px] [&>svg]:shrink-0",
  );
}

function Entry({
  entry,
  mode,
  onNavigate,
}: {
  entry: NavEntry;
  mode: SidebarMode;
  onNavigate: () => void;
}) {
  const Icon = entry.icon;
  const className = (active: boolean) => entryClassName(mode, active);

  return (
    <NavLink
      to={entry.to}
      end={entry.matchDescendants !== true}
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

/**
 * LogoutEntry はログアウトの項目である。確認の窓は出さず、成功したら今の URL を
 * ページごと読み直す。読み直した画面はゲストとして描かれる
 * （specs/016-single-account-auth/ui-design.md「Sidebar」）。
 */
function LogoutEntry({ mode }: { mode: SidebarMode }) {
  const toast = useToast();
  const [pending, setPending] = useState(false);

  const signOut = async () => {
    setPending(true);
    try {
      await logout();
      reloadPage();
    } catch {
      toast("ログアウトできませんでした");
      setPending(false);
    }
  };

  return (
    <button
      type="button"
      onClick={() => void signOut()}
      disabled={pending}
      className={cn(entryClassName(mode, false), "w-full disabled:opacity-50")}
    >
      <LogOut strokeWidth={1.75} />
      <span className="max-w-full truncate">
        {pending ? "ログアウト中…" : "ログアウト"}
      </span>
    </button>
  );
}

/** AccountEntries はサイドバーの下段の「アカウントと設定」である。 */
function AccountEntries({
  mode,
  onNavigate,
}: {
  mode: SidebarMode;
  onNavigate: () => void;
}) {
  const audience = useAudience();
  const location = useLocation();

  if (audience === "guest") {
    const loginEntry: NavEntry = {
      id: "login",
      label: "ログイン",
      icon: LogIn,
      to: loginPath(currentPath(location)),
    };
    return <Entry entry={loginEntry} mode={mode} onNavigate={onNavigate} />;
  }
  return (
    <>
      <Entry entry={settingsEntry} mode={mode} onNavigate={onNavigate} />
      <LogoutEntry mode={mode} />
    </>
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
  const audience = useAudience();
  const entries =
    audience === "owner" ? navEntries : navEntries.filter((entry) => !entry.ownerOnly);

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
          {entries.map((entry) => (
            <Entry key={entry.id} entry={entry} mode={mode} onNavigate={onClose} />
          ))}
        </nav>
        <nav
          aria-label="アカウントと設定"
          className={cn(
            "flex shrink-0 flex-col gap-0.5 border-t border-border",
            mode === "rail" ? "items-center px-1.5 py-2" : "px-2.5 py-3",
          )}
        >
          <AccountEntries mode={mode} onNavigate={onClose} />
        </nav>
      </aside>
    </>
  );
}
