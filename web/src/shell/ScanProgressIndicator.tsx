import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useNavigate } from "react-router";

import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { useScanNotice } from "./ScanNoticeProvider";
import ScanProgressBar from "./ScanProgressBar";
import { useScan } from "./ScanProvider";
import { presentScan, type ScanPresentation } from "./scanPresentation";

const POINTER_CLOSE_DELAY_MS = 100;

function iconFor(state: ScanPresentation["state"]) {
  if (state === "done") return CheckCircle2;
  if (state === "partial-failed") return AlertTriangle;
  if (state === "failed") return XCircle;
  return RefreshCw;
}

function countSummary(presentation: ScanPresentation) {
  const total = presentation.total === null ? "確認中" : String(presentation.total);
  return `${String(presentation.completed)} / ${total} 件（${String(presentation.failed)} 件失敗）`;
}

function statusAnnouncement(presentation: ScanPresentation) {
  switch (presentation.state) {
    case "starting":
      return "取り込みを開始しています";
    case "unknown-total":
      return "取り込み中。総件数を確認しています";
    case "running":
      return `取り込み対象は ${String(presentation.total)} 件です`;
    case "done":
      return "取り込みが完了しました";
    case "partial-failed":
      return "取り込みが一部失敗で完了しました";
    case "failed":
      return "取り込みに失敗しました";
    case "not-run":
    case "fetch-failed":
      return "";
  }
}

export default function ScanProgressIndicator() {
  const scan = useScan();
  const notice = useScanNotice();
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
    if (scan.loaded) notice.setCompletionNoticePaused(false);
  }, [notice.setCompletionNoticePaused, scan.loaded, visible]);

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
    notice.setCompletionNoticePaused(paused);
  }, [
    focused,
    notice.setCompletionNoticePaused,
    pointerActive,
    scan.loaded,
    terminalVisible,
  ]);

  if (!visible) return null;

  const Icon = iconFor(presentation.state);
  const description =
    presentation.state === "failed"
      ? "取り込みに失敗しました。設定で理由を確認してください"
      : presentation.description;
  const label =
    presentation.state === "starting"
      ? "開始中"
      : presentation.state === "running" || presentation.state === "unknown-total"
        ? presentation.progress === null
          ? "取り込み中"
          : `取り込み中 ${String(Math.round(presentation.progress * 100))}%`
        : presentation.state === "partial-failed"
          ? "一部失敗"
          : presentation.state === "failed"
            ? "取り込みに失敗しました"
            : "完了";
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
        // 再生画面でも右下に置く。右上は、幅によってプレイヤーの上か関連動画の見出しの行に
        // 閉じる × があり、そこを覆ってしまう。
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
              aria-label={`${label}。取り込み状況を開く`}
              className="inline-flex max-w-full items-center gap-2 rounded-md border border-border-strong bg-elevated px-3 py-2 text-sm text-fg shadow-elevated"
            >
              <Icon
                className={`size-4 shrink-0 ${presentation.state === "running" || presentation.state === "unknown-total" || presentation.state === "starting" ? "animate-spin motion-reduce:animate-none" : ""}`}
              />
              <span className="min-w-0 break-words tabular-nums">{label}</span>
            </button>
          </PopoverTrigger>
          {presentation.state === "failed" && (
            <IconButton
              label="取り込み失敗の通知を閉じる"
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
              <strong>{label}</strong>
              <span className="tabular-nums text-fg-muted">
                {countSummary(presentation)}
              </span>
            </div>
            <ScanProgressBar presentation={presentation} className="h-1.5" />
            <p className="text-sm text-fg-muted">{description}</p>
          </div>
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
