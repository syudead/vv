import { Pause } from "lucide-react";
import { type MouseEvent, useEffect } from "react";
import { useNavigate } from "react-router";

import type { BackgroundGroup } from "../api/background";
import { formatNumber, t } from "../i18n";
import BackgroundPauseSwitch from "../settings/BackgroundPauseSwitch";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { BackgroundCounts } from "./BackgroundCounts";
import { useBackground } from "./BackgroundProvider";
import { useIndicatorPopover } from "./useIndicatorPopover";

/** What a resume starts: work held back by a feature that is off does not count. */
export function pausedTotal(groups: readonly BackgroundGroup[]): number {
  return groups.reduce(
    (sum, group) => (group.featureOff === true ? sum : sum + group.remaining),
    0,
  );
}

/**
 * Stands in for the scan indicator on the owner's screens while background work is paused, so a
 * stopped library is not forgotten: it has no hide button and a stored dismissal does not hide it.
 */
export default function BackgroundPausedIndicator() {
  const source = useBackground();
  const navigate = useNavigate();
  const popover = useIndicatorPopover();
  const { reset } = popover;
  const status = source?.settings ?? null;
  const shown = status?.paused === true;

  useEffect(() => {
    if (!shown) reset();
  }, [reset, shown]);

  if (source === null || status === null || !shown) return null;

  const text = t.shell.background;
  const total = pausedTotal(status.groups);
  const goToSettings = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    popover.close();
    void navigate("/settings/scan#background-work");
  };

  return (
    // Raised above a list's selection bar, as notices are, so its actions stay uncovered.
    <div
      className="fixed right-3 bottom-indicator z-30 max-w-viewport-inset sm:right-5 sm:bottom-indicator-sm"
      onPointerEnter={popover.enterPointerArea}
      onPointerLeave={popover.leavePointerArea}
    >
      <Popover
        open={popover.open}
        onOpenChange={(next) => (next ? popover.setOpen(true) : popover.close())}
      >
        <div className="flex max-w-full items-center rounded-md border border-input bg-popover shadow-elevated">
          <PopoverTrigger asChild>
            <Button
              ref={popover.trigger}
              variant="ghost"
              onClick={goToSettings}
              onFocus={popover.focusTrigger}
              onBlur={popover.blurTrigger}
              onKeyDown={popover.keyDownTrigger}
              aria-label={text.indicatorName(total)}
              className="min-w-0 shrink font-normal"
            >
              <Pause aria-hidden="true" className="text-warning" />
              <span className="font-medium">{text.paused}</span>
              {total > 0 && (
                <span className="tabular-nums">
                  <span className="sm:hidden">{formatNumber(total)}</span>
                  <span className="hidden sm:inline">{text.left(total)}</span>
                </span>
              )}
            </Button>
          </PopoverTrigger>
        </div>
        <PopoverContent
          ref={popover.content}
          align="end"
          onOpenAutoFocus={(event) => event.preventDefault()}
          // Do not restore trigger focus after closing a hover-opened summary.
          onCloseAutoFocus={(event) => event.preventDefault()}
          onPointerEnter={popover.enterPointerArea}
          onPointerLeave={popover.leavePointerArea}
          onBlur={popover.blurContent}
          onKeyDown={popover.keyDownContent}
        >
          <div className="flex flex-col gap-3">
            <p className="inline-flex items-center gap-2 font-semibold">
              <Pause aria-hidden="true" className="size-4 text-warning" />
              {text.summaryHeading}
            </p>
            <BackgroundCounts groups={status.groups} />
            <BackgroundPauseSwitch
              source={source}
              status={status}
              id="background-pause-summary"
            />
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
