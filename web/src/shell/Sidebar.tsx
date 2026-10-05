import { LogIn, LogOut, Settings } from "lucide-react";
import { useState } from "react";
import { matchPath, NavLink, useLocation } from "react-router";

import { logout } from "../api/auth";
import { useAudience } from "../auth/audience";
import { currentPath, loginPath, reloadPage } from "../auth/pageNavigation";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import {
  Sidebar as SidebarRoot,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "../ui/shadcn/sidebar";
import { useToast } from "../ui/Toast";
import { navEntries, type NavEntry } from "./navigation";

function settingsEntry(): NavEntry {
  return { id: "settings", label: t.shell.nav.settings, icon: Settings, to: "/settings" };
}

/** useCloseDrawer は狭い幅のドロワーを閉じる関数を返す。ドロワーでなければ何もしない。 */
function useCloseDrawer(): () => void {
  const { isMobile, setOpenMobile } = useSidebar();
  return () => {
    if (isMobile) setOpenMobile(false);
  };
}

function Entry({ entry }: { entry: NavEntry }) {
  const location = useLocation();
  const closeDrawer = useCloseDrawer();
  const Icon = entry.icon;
  const end = entry.matchDescendants !== true;
  const active =
    matchPath({ path: entry.to.split("?")[0] ?? entry.to, end }, location.pathname) !==
    null;

  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active}>
        <NavLink to={entry.to} end={end} onClick={closeDrawer}>
          <Icon aria-hidden="true" />
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
      <SidebarMenuButton onClick={() => void signOut()} disabled={pending}>
        <LogOut aria-hidden="true" />
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

// レールでは 68px の幅に 56px の項目（アイコンの下に名前）を中央に置く。
const railPadding = "group-data-[collapsible=icon]:px-1.5";
const railItems = "group-data-[collapsible=icon]:items-center";

/**
 * Sidebar は左のナビである。shadcn/ui の Sidebar の 3 態を使う: 1024px 以上は展開
 * （閉じるとアイコンの下に名前を出す 68px のレール）、640–1023px は既定がレール、639px 以下は Sheet のドロワー
 * （web/registry/rules/components.md「Sidebar」）。開閉は AppShell の SidebarProvider が持つ。
 * 上段は画面の移動、下段は「アカウントと設定」で、それぞれ別の nav にする。
 */
export default function Sidebar() {
  const audience = useAudience();
  const all = navEntries();
  const entries = audience === "owner" ? all : all.filter((entry) => !entry.ownerOnly);

  return (
    // 上端はトップバーの下から始める（トップバーが全幅にあるため）。狭い幅のドロワーと
    // その背面の幕も同じで、トップバーを覆わない。ドロワー（Sheet）は置き場を
    // data-[side=left]: で決めるので、同じ印を付けてそれより優先させる（広い幅の入れ物も
    // data-side を持つため、どちらの幅にも効く）。
    <SidebarRoot
      collapsible="icon"
      className="data-[side=left]:top-navbar data-[side=left]:h-auto"
      overlayClassName="top-navbar"
    >
      <aside aria-label={t.shell.nav.main} className="flex min-h-0 flex-1 flex-col">
        <SidebarContent>
          <SidebarGroup className={railPadding}>
            <nav>
              <SidebarMenu className={railItems}>
                {entries.map((entry) => (
                  <Entry key={entry.id} entry={entry} />
                ))}
              </SidebarMenu>
            </nav>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter className={cn("border-t border-border", railPadding)}>
          <nav aria-label={t.shell.nav.account}>
            <SidebarMenu className={railItems}>
              <AccountEntries />
            </SidebarMenu>
          </nav>
        </SidebarFooter>
      </aside>
    </SidebarRoot>
  );
}
