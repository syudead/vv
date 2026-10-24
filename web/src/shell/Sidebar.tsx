import { LogIn, LogOut, Settings, SquareDashed } from "lucide-react";
import { useState } from "react";
import { matchPath, NavLink, useLocation } from "react-router";

import { logout } from "../api/auth";
import { useActivityTotals } from "../api/useActivityTotals";
import { useAudience } from "../auth/audience";
import { currentPath, loginPath, reloadPage } from "../auth/pageNavigation";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import NewVirtualVideoDialog from "../library/NewVirtualVideoDialog";
import {
  Sidebar as SidebarRoot,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuCount,
  SidebarMenuItem,
  useSidebar,
} from "../ui/shadcn/sidebar";
import { useToast } from "../ui/Toast";
import { navEntries, type NavEntry } from "./navigation";

function settingsEntry(): NavEntry {
  return {
    id: "settings",
    label: t.shell.nav.settings,
    icon: Settings,
    to: "/settings",
    matchDescendants: true,
  };
}

function useCloseDrawer(): () => void {
  const { isMobile, setOpenMobile } = useSidebar();
  return () => {
    if (isMobile) setOpenMobile(false);
  };
}

function Entry({ entry, count }: { entry: NavEntry; count?: number }) {
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
      {count !== undefined && (
        <SidebarMenuCount
          count={count}
          text={t.shell.nav.railCount(count)}
          label={t.shell.nav.activityCount(count)}
        />
      )}
    </SidebarMenuItem>
  );
}

/** Reload the current URL after logout to clear owner state and render as guest. */
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

/** A rarely used owner action, kept at the bottom of the upper area rather than in a page header. */
function NewVirtualVideoEntry({ onOpen }: { onOpen: () => void }) {
  const closeDrawer = useCloseDrawer();
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        onClick={() => {
          closeDrawer();
          onOpen();
        }}
      >
        <SquareDashed aria-hidden="true" />
        <span>{t.library.newVirtualVideo.action}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

const railPadding = "group-data-[collapsible=icon]:px-1.5";
const railItems = "group-data-[collapsible=icon]:items-center";

export default function Sidebar() {
  const audience = useAudience();
  const all = navEntries();
  const entries = audience === "owner" ? all : all.filter((entry) => !entry.ownerOnly);
  // Only owners fetch attention counts; activity events refresh them on every page.
  const totals = useActivityTotals(audience === "owner");
  const location = useLocation();
  // Held outside the drawer, which unmounts its content when it closes.
  const [creatingFrom, setCreatingFrom] = useState<string | null>(null);

  return (
    <>
      {/* Keep both drawer and backdrop below the full-width top bar; match Sheet's data-side specificity for narrow and wide layouts. */}
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
                    <Entry
                      key={entry.id}
                      entry={entry}
                      {...(entry.attentionCount === true && totals !== null
                        ? { count: totals.attention }
                        : {})}
                    />
                  ))}
                </SidebarMenu>
              </nav>
            </SidebarGroup>
            {audience === "owner" && (
              <SidebarGroup className={cn("mt-auto", railPadding)}>
                <SidebarMenu className={railItems}>
                  <NewVirtualVideoEntry
                    onOpen={() => setCreatingFrom(currentPath(location))}
                  />
                </SidebarMenu>
              </SidebarGroup>
            )}
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
      {creatingFrom !== null && (
        <NewVirtualVideoDialog
          backTo={creatingFrom}
          onClose={() => setCreatingFrom(null)}
        />
      )}
    </>
  );
}
