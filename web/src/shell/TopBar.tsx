import { Menu, RefreshCw } from "lucide-react";

import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import BrandHomeLink from "../ui/BrandHomeLink";
import { useSidebar } from "../ui/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";
import { useScan } from "./ScanProvider";

function ScanButton() {
  const scan = useScan();
  const buttonDescription = scan.canStart
    ? scan.running
      ? t.shell.topBar.scanning
      : t.shell.topBar.refreshLibrary
    : t.shell.topBar.needsMediaFolder;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={scan.start}
          disabled={scan.running || !scan.canStart}
          aria-label={buttonDescription}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-sm transition-colors select-none",
            scan.error !== null
              ? "bg-destructive-soft text-destructive"
              : scan.running
                ? "bg-primary-soft text-primary"
                : "text-foreground hover:bg-accent active:bg-secondary",
          )}
        >
          <RefreshCw
            className={cn(
              "size-4",
              scan.running && "animate-spin motion-reduce:animate-none",
            )}
          />
          <span className="hidden md:inline">
            {scan.running ? t.shell.topBar.refreshing : t.shell.topBar.refresh}
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{buttonDescription}</TooltipContent>
    </Tooltip>
  );
}

/**
 * TopBar は ☰・ロゴ・更新だけを持つ。ナビと設定は Sidebar にある。☰ は SidebarProvider の
 * toggleSidebar で、広い幅では展開とレール、狭い幅ではドロワーの開閉を切り替える。
 *
 * ゲストには更新を出さない。右端の入れ物ごと省き、道具の入れ物（flex-1）が右へ
 * 広がる。空の場所埋めは置かない（specs/016-single-account-auth/ui-design.md「Top bar」）。
 */
export default function TopBar() {
  const owner = useAudience() === "owner";
  const { toggleSidebar } = useSidebar();
  return (
    <header className="fixed inset-x-0 top-0 z-40 flex h-navbar items-center gap-1 border-b border-border bg-navbar px-2 sm:px-3">
      <IconButton label={t.shell.nav.menu} onClick={toggleSidebar} tooltip={false}>
        <Menu />
      </IconButton>
      <BrandHomeLink className="mr-1" />

      <div id="topbar-library-tools" className="flex min-w-0 flex-1 items-center" />

      {owner && (
        <div className="ml-auto flex shrink-0 items-center gap-0.5">
          <ScanButton />
        </div>
      )}
    </header>
  );
}
