import { Menu } from "lucide-react";

import AssistantTrigger from "../assistant/AssistantTrigger";
import { t } from "../i18n";
import BrandHomeLink from "../ui/BrandHomeLink";
import { Button } from "../ui/shadcn/button";
import { useSidebar } from "../ui/shadcn/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/** Hide the toggle on desktop because the rail never expands. */
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

export default function TopBar() {
  return (
    <header className="fixed inset-x-0 top-0 z-40 flex h-navbar items-center gap-2 border-b border-border bg-navbar px-2 sm:px-3">
      <MenuButton />
      <BrandHomeLink />

      <div id="topbar-library-tools" className="flex min-w-0 flex-1 items-center" />
      <AssistantTrigger />
    </header>
  );
}
