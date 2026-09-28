import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useNavigate } from "react-router";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { useScanNotice } from "./ScanNoticeProvider";
import ScanProgressBar from "./ScanProgressBar";
import { useScan } from "./ScanProvider";
import ProcessingBreakdown from "./ProcessingBreakdown";
import { presentScan, type ScanPresentation } from "./scanPresentation";

const POINTER_CLOSE_DELAY_MS = 100;

function iconFor(state: ScanPresentation["state"]) {
  if (state === "done") return CheckCircle2;
  if (state === "partial-failed") return AlertTriangle;
  if (state === "failed") return XCircle;
  return RefreshCw;
}

function countSummary(presentation: ScanPresentation): UiText {
  if (presentation.state === "preparing") {
    return presentation.processing === null
      ? t.shell.scan.checkingRemaining
      : t.shell.scan.remaining(presentation.remaining);
  }
  return t.shell.scan.counts(
    presentation.completed,
    presentation.total,
    presentation.failed,
  );
}

function statusAnnouncement(presentation: ScanPresentation): UiText | null {
  const announce = t.shell.scan.announce;
  switch (presentation.state) {
    case "starting":
      return announce.starting;
    case "unknown-total":
      return announce.unknownTotal;
    case "running":
      return announce.running(presentation.total ?? 0);
    case "preparing":
      return announce.preparing;
    case "done":
      return announce.done;
    case "partial-failed":
      return announce.partialFailed;
    case "failed":
      return announce.failed;
    case "not-run":
    case "fetch-failed":
      return null;
  }
}

function indicatorLabel(presentation: ScanPresentation): UiText {
  const label = t.shell.scan.label;
  switch (presentation.state) {
    case "starting":
      return label.starting;
    case "running":
    case "unknown-total":
      return presentation.progress === null
        ? label.scanning
        : label.scanningPercent(Math.round(presentation.progress * 100));
    case "preparing":
      return presentation.processing === null
        ? label.preparing
        : label.preparingLeft(presentation.remaining);
    case "partial-failed":
      return label.partialFailed;
    case "failed":
      return label.failed;
    default:
      return label.done;
  }
}

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
  const visible =
    presentation.state === "starting" ||
    presentation.state === "unknown-total" ||
    presentation.state === "running" ||
    presentation.state === "preparing" ||
    terminalVisible;

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

  const Icon = iconFor(presentation.state);
  const description =
    presentation.state === "failed"
      ? t.shell.scan.failedSeeSettings
      : presentation.description;
  const label = indicatorLabel(presentation);
  const goToDetails = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    navigatingToDetails.current = true;
    setOpen(false);
    if (presentation.state === "failed") notice.acknowledgeTerminalScan();
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
              aria-label={t.shell.scan.openStatus(label)}
              className="inline-flex max-w-full items-center gap-2 rounded-md border border-border-strong bg-elevated px-3 py-2 text-sm text-fg shadow-elevated"
            >
              <Icon
                className={`size-4 shrink-0 ${presentation.state === "running" || presentation.state === "unknown-total" || presentation.state === "starting" || presentation.state === "preparing" ? "animate-spin motion-reduce:animate-none" : ""}`}
              />
              <span className="min-w-0 break-words tabular-nums">{label}</span>
            </button>
          </PopoverTrigger>
          {presentation.state === "failed" && (
            <IconButton
              label={t.shell.scan.dismissFailure}
              size="sm"
              className="-ml-1 rounded-md border border-border-strong bg-elevated shadow-elevated"
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
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <strong>
                {presentation.state === "preparing"
                  ? t.shell.scan.label.preparing
                  : label}
              </strong>
              <span className="tabular-nums text-fg-muted">
                {countSummary(presentation)}
              </span>
            </div>
            <ScanProgressBar presentation={presentation} className="h-1.5" />
            {/* 準備中は件数を見出しと内訳に出しているので、同じ件数の説明文を重ねない。 */}
            {presentation.state === "preparing" ? (
              <p className="text-sm text-fg-muted">{t.shell.scan.announce.preparing}</p>
            ) : (
              <p className="text-sm text-fg-muted">{description}</p>
            )}
            <ProcessingBreakdown
              processing={presentation.processing}
              className="text-xs text-fg-muted"
            />
          </div>
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
