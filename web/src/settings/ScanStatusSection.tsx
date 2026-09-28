import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

import { formatDateTime, formatNumber, scanErrorText, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import ScanProgressBar from "../shell/ScanProgressBar";
import { useScan } from "../shell/ScanProvider";
import ProcessingBreakdown from "../shell/ProcessingBreakdown";
import { presentScan, type ScanPresentation } from "../shell/scanPresentation";

function formatTime(value?: string): UiText {
  if (value === undefined) return t.settings.scanStatus.notFinished;
  return formatDateTime(value);
}

function stateLabel(presentation: ScanPresentation): UiText {
  const state = t.settings.scanStatus.state;
  switch (presentation.state) {
    case "not-run":
      return state.notRun;
    case "starting":
      return state.starting;
    case "unknown-total":
    case "running":
      return state.running;
    case "preparing":
      return state.preparing;
    case "done":
      return state.done;
    case "partial-failed":
      return state.partialFailed;
    case "failed":
      return state.failed;
    case "fetch-failed":
      return state.fetchFailed;
  }
}

/**
 * failureText は取り込みの失敗の説明である。`Scan.error`（自由文）は出さず、
 * `errorCode` と `errorPath` から作る。スキャンが無いときは状態取得の失敗を出す。
 */
function failureText(presentation: ScanPresentation): UiText {
  if (presentation.scan !== null) return scanErrorText(presentation.scan);
  return presentation.error ?? t.errors.scanUnknown;
}

function StateIcon({ state }: { state: ScanPresentation["state"] }) {
  if (state === "done") return <CheckCircle2 className="size-4" />;
  if (state === "partial-failed") return <AlertTriangle className="size-4" />;
  if (state === "failed") return <XCircle className="size-4" />;
  return <RefreshCw className="size-4" />;
}

export default function ScanStatusSection() {
  const location = useLocation();
  const heading = useRef<HTMLHeadingElement>(null);
  const scan = useScan();
  const presentation = presentScan(scan);
  const text = t.settings.scanStatus;
  const retry = () => scan.start();
  const empty = presentation.state === "not-run";
  const noItems = presentation.state === "done" && presentation.total === 0;

  useLayoutEffect(() => {
    if (location.hash !== "#scan-status") return;
    const focusHeading = () => {
      heading.current?.scrollIntoView?.({ block: "start" });
      heading.current?.focus();
    };
    focusHeading();
    const raf = window.requestAnimationFrame(focusHeading);
    const timers = [
      window.setTimeout(focusHeading, 0),
      window.setTimeout(focusHeading, 100),
    ];
    return () => {
      window.cancelAnimationFrame(raf);
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [location.hash, location.key]);

  return (
    <section
      id="scan-status"
      aria-labelledby="scan-status-heading"
      className="mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5"
    >
      <div>
        <div className="flex flex-wrap items-center gap-2">
          <h2
            ref={heading}
            id="scan-status-heading"
            tabIndex={-1}
            className="text-base font-semibold"
          >
            {text.heading}
          </h2>
          <span className="inline-flex items-center gap-1 rounded-md bg-elevated px-2 py-1 text-xs text-fg-muted">
            <StateIcon state={presentation.state} />
            {stateLabel(presentation)}
          </span>
        </div>
        <p className="mt-3 text-sm leading-6 text-fg-muted">
          {empty
            ? t.shell.scan.notRun
            : noItems
              ? text.nothingFound
              : presentation.description}
        </p>
        <div className="mt-4 max-w-xl">
          <ScanProgressBar presentation={presentation} className="h-2" />
        </div>
        <dl className="mt-4 grid grid-cols-1 gap-2 text-sm text-fg-muted sm:grid-cols-3">
          <div>
            <dt>{text.processed}</dt>
            <dd className="tabular-nums text-fg">
              {formatNumber(presentation.completed)}
            </dd>
          </div>
          <div>
            <dt>{text.total}</dt>
            <dd className="tabular-nums text-fg">
              {presentation.total === null
                ? text.counting
                : formatNumber(presentation.total)}
            </dd>
          </div>
          <div>
            <dt>{text.failed}</dt>
            <dd className="tabular-nums text-fg">{formatNumber(presentation.failed)}</dd>
          </div>
        </dl>
        {presentation.remaining > 0 && (
          <div className="mt-4 text-sm text-fg-muted">
            <p>{text.preparationLeft}</p>
            <ProcessingBreakdown
              processing={presentation.processing}
              className="mt-2 max-w-xl"
            />
          </div>
        )}
        <dl className="mt-4 grid grid-cols-1 gap-2 text-sm text-fg-muted sm:grid-cols-2">
          <div>
            <dt>{text.startedAt}</dt>
            <dd className="text-fg">{formatTime(presentation.startedAt)}</dd>
          </div>
          <div>
            <dt>{text.finishedAt}</dt>
            <dd className="text-fg">{formatTime(presentation.finishedAt)}</dd>
          </div>
        </dl>
        {presentation.refreshing && (
          <p role="status" className="mt-3 text-sm text-warning">
            {text.rechecking}
          </p>
        )}
        {presentation.state === "failed" && (
          <div className="mt-4 flex flex-col items-start gap-3">
            <p className="break-words text-sm text-danger">{failureText(presentation)}</p>
            <Button
              variant="primary"
              onClick={retry}
              disabled={!scan.canStart || scan.running}
            >
              <RefreshCw />
              {t.common.retry}
            </Button>
          </div>
        )}
      </div>
    </section>
  );
}
