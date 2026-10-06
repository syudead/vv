import { Menu } from "lucide-react";

import { t } from "../i18n";
import BrandHomeLink from "../ui/BrandHomeLink";
import { Button } from "../ui/shadcn/button";
import { useSidebar } from "../ui/shadcn/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

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
 * TopBar は ☰（639px 以下）・ロゴ・ページの道具を持つ。ナビと設定は Sidebar にある。
 * ページの道具（一覧のツールバー）は TopBarPortal が中央の入れ物へ描く。
 *
 * 取り込みの開始と状態は出さない。開始は設定の「Scan status」、進み具合は右下の
 * 表示が受け持つ（issue 830）。
 */
export default function TopBar() {
  return (
    <header className="fixed inset-x-0 top-0 z-40 flex h-navbar items-center gap-2 border-b border-border bg-navbar px-2 sm:px-3">
      <MenuButton />
      <BrandHomeLink />

      <div id="topbar-library-tools" className="flex min-w-0 flex-1 items-center" />
    </header>
  );
}
