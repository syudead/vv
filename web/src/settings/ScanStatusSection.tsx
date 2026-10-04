import { RefreshCw } from "lucide-react";
import { useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

import { scanErrorText, t, type UiText } from "../i18n";
import Button from "../ui/legacy/Button";
import ScanProgressBar from "../shell/ScanProgressBar";
import { useScan } from "../shell/ScanProvider";
import { ScanDetail, ScanIssueCounts, ScanStatusIcon } from "../shell/ScanSummaryParts";
import { presentScan, type ScanPresentation } from "../shell/scanPresentation";
import ScanIssueList from "./ScanIssueList";
import { useScanIssues } from "./useScanIssues";

function stateLabel(presentation: ScanPresentation): UiText {
  if (presentation.statusText !== null) return presentation.statusText;
  return presentation.state === "fetch-failed"
    ? t.settings.scanStatus.state.fetchFailed
    : t.settings.scanStatus.state.notRun;
}

/**
 * failureText は取り込みの失敗の説明である。`Scan.error`（自由文）は出さず、
 * `errorCode` と `errorPath` から作る。スキャンが無いときは状態取得の失敗を出す。
 */
function failureText(presentation: ScanPresentation): UiText {
  if (presentation.scan !== null) return scanErrorText(presentation.scan);
  return presentation.error ?? t.errors.scanUnknown;
}

/**
 * ScanStatusSection は設定の「Scan status」の概要である（specs/024-import-progress/ui-design.md
 * 「Settings Scan Status」）。概要（右下の popover）と同じ要素に、分母の意味・取り込み
 * 自体の失敗の理由と再試行・問題の一覧だけを足す。
 */
export default function ScanStatusSection() {
  const location = useLocation();
  const heading = useRef<HTMLHeadingElement>(null);
  const scan = useScan();
  const presentation = presentScan(scan);
  const text = t.settings.scanStatus;
  const retry = () => scan.start();
  const issues = useScanIssues(scan.scan, scan.refresh);
  const issueCount = presentation.issues.failed + presentation.issues.substituted;

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
            <ScanStatusIcon state={presentation.state} />
            {stateLabel(presentation)}
          </span>
          {(presentation.issues.failed > 0 || presentation.issues.substituted > 0) && (
            <ScanIssueCounts
              failed={presentation.issues.failed}
              substituted={presentation.issues.substituted}
              className="rounded-md bg-elevated px-2 py-1 text-xs text-fg-muted"
            />
          )}
        </div>
        <div className="mt-4 max-w-xl space-y-2">
          <ScanProgressBar presentation={presentation} className="h-2" />
          {presentation.state === "not-run" ? (
            <p className="text-sm text-fg-muted">{t.shell.scan.notRun}</p>
          ) : presentation.state === "fetch-failed" ? (
            <p className="text-sm text-fg-muted">{presentation.error}</p>
          ) : (
            presentation.progressText !== null && (
              <p className="text-sm tabular-nums text-fg">{presentation.progressText}</p>
            )
          )}
          {presentation.videos !== null && (
            <p className="text-xs text-fg-muted">{t.shell.scan.denominator}</p>
          )}
          {presentation.state !== "not-run" && presentation.state !== "fetch-failed" && (
            <ScanDetail presentation={presentation} />
          )}
        </div>
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
        {(issueCount > 0 || issues.items.length > 0) && (
          <ScanIssueList
            list={issues}
            failed={presentation.issues.failed}
            substituted={presentation.issues.substituted}
          />
        )}
      </div>
    </section>
  );
}
