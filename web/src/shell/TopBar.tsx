import { Menu, RefreshCw } from "lucide-react";

import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import BrandHomeLink from "../ui/BrandHomeLink";
import { Button } from "../ui/shadcn/button";
import { useSidebar } from "../ui/shadcn/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
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
      {/* 押せない間も理由のツールチップを出せるよう、ボタンを包みに入れる。 */}
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <Button
            variant="ghost"
            size="sm"
            onClick={scan.start}
            disabled={scan.running || !scan.canStart}
            aria-label={buttonDescription}
            aria-busy={scan.running || undefined}
          >
            <RefreshCw
              aria-hidden="true"
              className={cn(
                scan.error !== null && "text-destructive",
                scan.running && "animate-spin text-primary motion-reduce:animate-none",
              )}
            />
            <span className="hidden md:inline">
              {scan.running ? t.shell.topBar.refreshing : t.shell.topBar.refresh}
            </span>
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{buttonDescription}</TooltipContent>
    </Tooltip>
  );
}

/**
 * MenuButton は 639px 以下でサイドバーのドロワーを開閉する。640px 以上のサイドバーは
 * いつもレールで開閉しないので、ボタンを出さない。
 */
function MenuButton() {
  const { toggleSidebar } = useSidebar();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="sm:hidden"
          aria-label={t.shell.nav.menu}
          onClick={toggleSidebar}
        >
          <Menu aria-hidden="true" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t.shell.nav.menu}</TooltipContent>
    </Tooltip>
  );
}

/**
 * TopBar は ☰（639px 以下）・ロゴ・ページの道具・更新を持つ。ナビと設定は Sidebar にある。
 * ページの道具（一覧のツールバー）は TopBarPortal が中央の入れ物へ描く。
 *
 * ゲストには更新を出さない。右端の入れ物ごと省き、道具の入れ物（flex-1）が右へ
 * 広がる。空の場所埋めは置かない（specs/016-single-account-auth/ui-design.md「Top bar」）。
 */
export default function TopBar() {
  const owner = useAudience() === "owner";
  return (
    <header className="fixed inset-x-0 top-0 z-40 flex h-navbar items-center gap-2 border-b border-border bg-navbar px-2 sm:px-3">
      <MenuButton />
      <BrandHomeLink />

      <div id="topbar-library-tools" className="flex min-w-0 flex-1 items-center" />

      {owner && (
        <div className="ml-auto flex shrink-0 items-center">
          <ScanButton />
        </div>
      )}
    </header>
  );
}
