import { XCircle } from "lucide-react";
import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useNavigate } from "react-router";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
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

/** indicatorName は右下の本体の名前（「Open the scan status」の前）である。 */
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
 * ScanProgressIndicator は右下の本体と概要である（specs/024-import-progress/ui-design.md
 * 「Floating Indicator」「Summary Popover」）。置き場所・開き方・押したときの移動先は
 * 012 の形のまま。
 */
export default function ScanProgressIndicator() {
  const scan = useScan();
  const notice = useScanNotice();
  const { setCompletionNoticePaused } = notice;
  const navigate = useNavigate();
  const presentation = presentScan(scan);
  const [open, setOpen] = useState(false);
  const [pointerActive, setPointerActive] = useState(false);
  const [focused, setFocused] = useState(false);
  const pointerCloseTimer = useRef<number | null>(null);
  const navigatingToDetails = useRef(false);

  const terminalVisible =
    presentation.scan !== null &&
    notice.completionNotice?.scanId === presentation.scan.id;
  const visible = inProgressState(presentation.state) || terminalVisible;
  // 一部失敗と失敗は、閉じる button で閉じるか設定へ移るまで残す。
  const persistent = presentation.state === "partial" || presentation.state === "failed";

  useEffect(() => {
    setOpen(pointerActive || focused);
  }, [focused, pointerActive]);

  useEffect(() => {
    if (visible) return;
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
    setPointerActive(false);
    setFocused(false);
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

  useEffect(() => {
    if (!scan.loaded) return;
    const paused = terminalVisible && (pointerActive || focused);
    setCompletionNoticePaused(paused);
  }, [focused, setCompletionNoticePaused, pointerActive, scan.loaded, terminalVisible]);

  if (!visible) return null;

  const goToDetails = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    navigatingToDetails.current = true;
    setOpen(false);
    if (persistent) notice.acknowledgeTerminalScan();
    navigate("/settings#scan-status");
  };
  const enterPointerArea = () => {
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
      pointerCloseTimer.current = null;
    }
    setPointerActive(true);
  };
  const leavePointerArea = () => {
    if (pointerCloseTimer.current !== null) {
      window.clearTimeout(pointerCloseTimer.current);
    }
    pointerCloseTimer.current = window.setTimeout(() => {
      pointerCloseTimer.current = null;
      setPointerActive(false);
    }, POINTER_CLOSE_DELAY_MS);
  };

  return (
    <div
      className={cn(
        "fixed right-3 z-30 max-w-[calc(100vw-1.5rem)] sm:right-5",
        // 再生画面でも右下に置く。右上には見出しの帯の閉じる × があり、そこを覆ってしまう。
        "bottom-4 sm:bottom-5",
      )}
      onPointerEnter={enterPointerArea}
      onPointerLeave={leavePointerArea}
    >
      <span role="status" aria-atomic="true" className="sr-only">
        {statusAnnouncement(presentation)}
      </span>
      <PopoverRoot open={open} onOpenChange={setOpen}>
        <div className="flex items-center gap-1">
          <PopoverTrigger asChild>
            <button
              type="button"
              onClick={goToDetails}
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              aria-label={t.shell.scan.openStatus(indicatorName(presentation))}
              className="inline-flex max-w-full items-center gap-2 whitespace-nowrap rounded-md border border-input bg-popover px-3 py-2 text-sm text-foreground shadow-elevated"
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
            </button>
          </PopoverTrigger>
          {persistent && (
            <IconButton
              label={t.shell.scan.dismissResult}
              size="sm"
              className="-ml-1 rounded-md border border-input bg-popover shadow-elevated"
              onClick={() => notice.acknowledgeTerminalScan()}
            >
              <XCircle />
            </IconButton>
          )}
        </div>
        <PopoverContent
          align="end"
          className="w-[min(18rem,calc(100vw-1rem))]"
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => {
            if (!navigatingToDetails.current) return;
            event.preventDefault();
            window.requestAnimationFrame(() => {
              navigatingToDetails.current = false;
            });
          }}
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
            <ScanProgressBar presentation={presentation} className="h-1.5" />
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
      </PopoverRoot>
    </div>
  );
}
