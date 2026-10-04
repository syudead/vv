import { LogIn, LogOut, Settings } from "lucide-react";
import { useState } from "react";
import { NavLink, useLocation } from "react-router";

import { logout } from "../api/auth";
import { useAudience } from "../auth/audience";
import { currentPath, loginPath, reloadPage } from "../auth/pageNavigation";
import { t } from "../i18n";
import { useToast } from "../ui/legacy/Toast";
import {
  Sidebar as SidebarRoot,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "../ui/sidebar";
import { navEntries, type NavEntry } from "./navigation";

/** closeDrawer は狭い幅のドロワーを閉じる。広い幅では何もしない（展開とレールは残す）。 */
function useCloseDrawer(): () => void {
  const { isMobile, setOpenMobile } = useSidebar();
  return () => {
    if (isMobile) setOpenMobile(false);
  };
}

function settingsEntry(): NavEntry {
  return { id: "settings", label: t.shell.nav.settings, icon: Settings, to: "/settings" };
}

function Entry({ entry }: { entry: NavEntry }) {
  const Icon = entry.icon;
  const closeDrawer = useCloseDrawer();
  const location = useLocation();
  const active =
    entry.matchDescendants === true
      ? location.pathname === entry.to || location.pathname.startsWith(`${entry.to}/`)
      : location.pathname === entry.to;

  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={entry.label}>
        <NavLink
          to={entry.to}
          end={entry.matchDescendants !== true}
          onClick={closeDrawer}
        >
          <Icon />
          <span>{entry.label}</span>
        </NavLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

/**
 * LogoutEntry はログアウトの項目である。確認の窓は出さず、成功したら今の URL を
 * ページごと読み直す。読み直した画面はゲストとして描かれる
 * （specs/016-single-account-auth/ui-design.md「Sidebar」）。
 */
function LogoutEntry() {
  const toast = useToast();
  const [pending, setPending] = useState(false);
  const label = pending ? t.shell.nav.loggingOut : t.shell.nav.logout;

  const signOut = async () => {
    setPending(true);
    try {
      await logout();
      reloadPage();
    } catch {
      toast(t.shell.nav.logoutFailed);
      setPending(false);
    }
  };

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        onClick={() => void signOut()}
        disabled={pending}
        tooltip={label}
      >
        <LogOut />
        <span>{label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

/** AccountEntries はサイドバーの下段の「アカウントと設定」である。 */
function AccountEntries() {
  const audience = useAudience();
  const location = useLocation();

  if (audience === "guest") {
    const loginEntry: NavEntry = {
      id: "login",
      label: t.shell.nav.login,
      icon: LogIn,
      to: loginPath(currentPath(location)),
    };
    return <Entry entry={loginEntry} />;
  }
  return (
    <>
      <Entry entry={settingsEntry()} />
      <LogoutEntry />
    </>
  );
}

/**
 * Sidebar は左のナビである。ui/sidebar（shadcn/ui の Sidebar）の 3 つの形を使う:
 * 広い幅の展開、畳んだレール（collapsible="icon"。名前はツールチップに出す）、
 * 狭い幅のドロワー（Sheet）。上段はナビ、下段はアカウントと設定。
 */
export default function Sidebar() {
  const audience = useAudience();
  const all = navEntries();
  const entries = audience === "owner" ? all : all.filter((entry) => !entry.ownerOnly);

  return (
    <SidebarRoot collapsible="icon" className="top-navbar bottom-0 h-auto">
      <aside aria-label={t.shell.nav.main} className="flex min-h-0 flex-1 flex-col">
        <SidebarContent>
          <SidebarGroup>
            <nav>
              <SidebarMenu>
                {entries.map((entry) => (
                  <Entry key={entry.id} entry={entry} />
                ))}
              </SidebarMenu>
            </nav>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter className="border-t border-border">
          <nav aria-label={t.shell.nav.account}>
            <SidebarMenu>
              <AccountEntries />
            </SidebarMenu>
          </nav>
        </SidebarFooter>
      </aside>
    </SidebarRoot>
  );
}
