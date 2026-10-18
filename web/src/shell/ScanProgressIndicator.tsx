import { X } from "lucide-react";
import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useNavigate } from "react-router";

import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useScanNotice } from "./ScanNoticeProvider";
import ScanProgressBar from "./ScanProgressBar";
import { useScan } from "./ScanProvider";
import { ScanDetail, ScanIssueCounts, ScanStatusIcon } from "./ScanSummaryParts";
import {
  inProgressState,
  issueCountTexts,
  presentScan,
  statusAnnouncement,
  type ScanPresentation,
} from "./scanPresentation";

const POINTER_CLOSE_DELAY_MS = 100;

function indicatorName(presentation: ScanPresentation): UiText {
  const text = t.shell.scan;
  const issues = issueCountTexts(presentation);
  return text.indicatorName(
    presentation.statusText ?? text.status.starting,
    presentation.videos === null
      ? null
      : text.videosDone(presentation.videos.settled, presentation.videos.total),
    issues.length === 0 ? null : text.issueCounts(issues),
  );
}

/**
 * Drive popup visibility from open/close events, not hover/focus state, so restored focus cannot
 * reopen it. Dismissal suppresses this scan and its result, but not the next scan.
 */
export default function ScanProgressIndicator() {
  const scan = useScan();
  const notice = useScanNotice();
  const { setCompletionNoticePaused } = notice;
  const navigate = useNavigate();
  const presentation = presentScan(scan);
  const [open, setOpen] = useState(false);
  const pointerCloseTimer = useRef<number | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const pointerInside = useRef(false);
  // After closing while focused, require focus to leave before it can reopen the popup.
  const focusOpenSuppressed = useRef(false);

  const terminalVisible =
    presentation.scan !== null &&
    notice.completionNotice?.scanId === presentation.scan.id;
  // Starting still references the previous scan; its dismissal must not hide the new start.
  const dismissed =
    notice.dismissPending ||
    (presentation.state !== "starting" &&
      presentation.scan !== null &&
      presentation.scan.id === notice.dismissedScanId);
  // Hide running automatic scans; ScanNoticeProvider decides whether terminal failures warrant a result.
  const visible =
    !dismissed &&
    ((inProgressState(presentation.state) && !presentation.watch) || terminalVisible);

  const clearPointerCloseTimer = () => {
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
  };

  useEffect(() => {
    if (visible) return;
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
    pointerInside.current = false;
    focusOpenSuppressed.current = false;
    setOpen(false);
    if (scan.loaded) setCompletionNoticePaused(false);
  }, [scan.loaded, setCompletionNoticePaused, visible]);

  useEffect(
    () => () => {
      if (pointerCloseTimer.current !== null) {
        window.clearTimeout(pointerCloseTimer.current);
      }
    },
    [],
  );

  // Pause result expiry only while the popup is open.
  useEffect(() => {
    if (!scan.loaded) return;
    setCompletionNoticePaused(terminalVisible && open);
  }, [open, scan.loaded, setCompletionNoticePaused, terminalVisible]);

  if (!visible) return null;

  const close = () => {
    clearPointerCloseTimer();
    focusOpenSuppressed.current = document.activeElement === trigger.current;
    setOpen(false);
  };
  const goToDetails = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    close();
    if (terminalVisible) notice.acknowledgeTerminalScan();
    navigate("/settings/scan#scan-status");
  };
  const enterPointerArea = () => {
    clearPointerCloseTimer();
    pointerInside.current = true;
    setOpen(true);
  };
  const leavePointerArea = () => {
    clearPointerCloseTimer();
    pointerCloseTimer.current = window.setTimeout(() => {
      pointerInside.current = false;
      close();
    }, POINTER_CLOSE_DELAY_MS);
  };
  const focusTrigger = () => {
    if (!focusOpenSuppressed.current) setOpen(true);
  };
  const blurTrigger = () => {
    focusOpenSuppressed.current = false;
    // Let pointer handling close while the pointer remains over the trigger or popup.
    if (!pointerInside.current) setOpen(false);
  };
  const dismissIndicator = () => {
    close();
    notice.dismissIndicator();
  };

  return (
    // Keep bottom-right on playback to avoid covering its top-right close button.
    <div
      className="fixed right-3 bottom-4 z-30 max-w-viewport-inset sm:right-5 sm:bottom-5"
      onPointerEnter={enterPointerArea}
      onPointerLeave={leavePointerArea}
    >
      <span role="status" aria-atomic="true" className="sr-only">
        {statusAnnouncement(presentation)}
      </span>
      <Popover open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
        <div className="flex max-w-full items-center rounded-md border border-input bg-popover shadow-elevated">
          <PopoverTrigger asChild>
            <Button
              ref={trigger}
              variant="ghost"
              onClick={goToDetails}
              onFocus={focusTrigger}
              onBlur={blurTrigger}
              aria-label={t.shell.scan.openStatus(indicatorName(presentation))}
              className="min-w-0 shrink font-normal"
            >
              <ScanStatusIcon state={presentation.state} />
              <span className="font-medium">{presentation.statusText}</span>
              {presentation.videos !== null && (
                <span className="tabular-nums">
                  {t.shell.scan.videosShort(
                    presentation.videos.settled,
                    presentation.videos.total,
                  )}
                </span>
              )}
              <ScanIssueCounts
                failed={presentation.issues.failed}
                substituted={presentation.issues.substituted}
                mostSevereOnly
                compact
              />
            </Button>
          </PopoverTrigger>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t.shell.scan.dismiss}
                className="mr-0.5 text-muted-foreground"
                onClick={dismissIndicator}
              >
                <X aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t.shell.scan.dismiss}</TooltipContent>
          </Tooltip>
        </div>
        <PopoverContent
          align="end"
          onOpenAutoFocus={(event) => event.preventDefault()}
          // Do not restore trigger focus after closing a hover-opened popup; persistent focus obscures the closed state.
          onCloseAutoFocus={(event) => event.preventDefault()}
          onPointerEnter={enterPointerArea}
          onPointerLeave={leavePointerArea}
        >
          <div className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
              <span className="inline-flex items-center gap-2 whitespace-nowrap font-semibold">
                <ScanStatusIcon state={presentation.state} />
                {presentation.statusText}
              </span>
              <ScanIssueCounts
                failed={presentation.issues.failed}
                substituted={presentation.issues.substituted}
                className="text-sm"
              />
            </div>
            <ScanProgressBar presentation={presentation} />
            {presentation.progressText !== null && (
              <p className="text-sm tabular-nums">{presentation.progressText}</p>
            )}
            <ScanDetail presentation={presentation} />
            {(presentation.state === "partial" ||
              (presentation.state === "done" && presentation.issues.substituted > 0)) && (
              <p className="text-xs text-muted-foreground">
                {t.shell.scan.seeSettingsForList}
              </p>
            )}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
