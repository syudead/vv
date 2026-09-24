import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";

import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { useScanNotice } from "./ScanNoticeProvider";
import { useScan } from "./ScanProvider";
import { presentScan, type ScanPresentation } from "./scanPresentation";

function ProgressBar({ presentation }: { presentation: ScanPresentation }) {
  if (!presentation.determinate || presentation.progress === null) return null;
  const value = Math.round(presentation.progress * 100);
  return (
    <div
      role="progressbar"
      aria-label="取り込みの進捗"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={value}
      className="h-1.5 overflow-hidden rounded-full bg-bg"
    >
      <div
        className="h-full bg-accent transition-[width] duration-300"
        style={{ width: `${String(value)}%` }}
      />
    </div>
  );
}

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

export default function ScanProgressIndicator() {
  const scan = useScan();
  const notice = useScanNotice();
  const location = useLocation();
  const navigate = useNavigate();
  const presentation = presentScan(scan);
  const [open, setOpen] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [contentHovered, setContentHovered] = useState(false);
  const [focused, setFocused] = useState(false);
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
    setOpen(hovered || contentHovered || focused);
  }, [contentHovered, focused, hovered]);

  useEffect(() => {
    const paused = terminalVisible && (hovered || contentHovered || focused);
    notice.setCompletionNoticePaused(paused);
    return () => {
      if (paused) notice.setCompletionNoticePaused(false);
    };
  }, [contentHovered, focused, hovered, notice, terminalVisible]);

  if (!visible) return null;

  const Icon = iconFor(presentation.state);
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
  const goToDetails = () => {
    navigatingToDetails.current = true;
    setOpen(false);
    navigate("/settings#scan-status");
  };

  return (
    <div
      className={cn(
        "fixed right-3 z-30 max-w-[calc(100vw-1.5rem)] sm:right-5",
        location.pathname.startsWith("/videos/") ? "top-2" : "bottom-4 sm:bottom-5",
      )}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
    >
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
          onCloseAutoFocus={(event) => {
            if (!navigatingToDetails.current) return;
            event.preventDefault();
            window.requestAnimationFrame(() => {
              navigatingToDetails.current = false;
            });
          }}
          onPointerEnter={() => setContentHovered(true)}
          onPointerLeave={() => setContentHovered(false)}
        >
          <div role="status" className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <strong>{label}</strong>
              <span className="tabular-nums text-fg-muted">
                {countSummary(presentation)}
              </span>
            </div>
            <ProgressBar presentation={presentation} />
            <p className="text-sm text-fg-muted">{presentation.description}</p>
          </div>
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
