import { WifiLow, X } from "lucide-react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/** A nonblocking status banner beneath controls and state overlays. Only its close button intercepts input. */
export default function StallWarning({
  onDismiss,
  container,
}: {
  onDismiss: () => void;
  /** Tooltips must render inside the fullscreen container to remain visible. */
  container: HTMLElement | null;
}) {
  const text = t.player.stallWarning;
  return (
    <div
      data-stall-warning=""
      className="pointer-events-none absolute inset-x-2 top-2 z-5 flex sm:inset-x-3 sm:top-3"
    >
      <div
        role="status"
        className="flex max-w-full items-center gap-2 rounded-md bg-navbar px-3 py-1.5 text-xs text-foreground shadow-elevated animate-fade-in motion-reduce:animate-none"
      >
        <WifiLow className="size-4 shrink-0 text-warning" aria-hidden="true" />
        <span className="min-w-0">{text.message}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={text.dismiss}
              onClick={onDismiss}
              className="pointer-events-auto -mr-1.5 size-6 rounded-sm text-muted-foreground [&_svg]:size-3"
            >
              <X aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent container={container}>{text.dismiss}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
