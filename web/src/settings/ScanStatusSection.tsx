import { CircleX, RefreshCw } from "lucide-react";
import { useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

import { scanErrorText, t, type UiText } from "../i18n";
import ScanProgressBar from "../shell/ScanProgressBar";
import { useScan } from "../shell/ScanProvider";
import { ScanDetail, ScanIssueCounts, ScanStatusIcon } from "../shell/ScanSummaryParts";
import { presentScan, type ScanPresentation } from "../shell/scanPresentation";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
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
  const anchor = useRef<HTMLDivElement>(null);
  const scan = useScan();
  const presentation = presentScan(scan);
  const text = t.settings.scanStatus;
  const retry = () => scan.start();
  const issues = useScanIssues(scan.scan, scan.refresh);
  const issueCount = presentation.issues.failed + presentation.issues.substituted;

  useLayoutEffect(() => {
    if (location.hash !== "#scan-status") return;
    // 節の見出し（h2）は PageSection が描く。移ってきたときだけ focus できるようにして移す。
    const focusHeading = () => {
      const heading = anchor.current?.querySelector<HTMLHeadingElement>("h2");
      if (heading === null || heading === undefined) return;
      heading.tabIndex = -1;
      heading.scrollIntoView?.({ block: "start" });
      heading.focus();
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
    <div ref={anchor} id="scan-status">
      <PageSection
        title={text.heading}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">
              <ScanStatusIcon state={presentation.state} />
              {stateLabel(presentation)}
            </Badge>
            {(presentation.issues.failed > 0 || presentation.issues.substituted > 0) && (
              <Badge variant="secondary">
                <ScanIssueCounts
                  failed={presentation.issues.failed}
                  substituted={presentation.issues.substituted}
                  className="inline-flex gap-2"
                />
              </Badge>
            )}
          </span>
        }
      >
        <div className="flex flex-col gap-2">
          <ScanProgressBar presentation={presentation} className="h-2" />
          {presentation.state === "not-run" ? (
            <p className="text-muted-foreground">{t.shell.scan.notRun}</p>
          ) : presentation.state === "fetch-failed" ? (
            <p className="text-muted-foreground">{presentation.error}</p>
          ) : (
            presentation.progressText !== null && (
              <p className="tabular-nums">{presentation.progressText}</p>
            )
          )}
          {presentation.videos !== null && (
            <p className="text-xs text-muted-foreground">{t.shell.scan.denominator}</p>
          )}
          {presentation.state !== "not-run" && presentation.state !== "fetch-failed" && (
            <ScanDetail presentation={presentation} />
          )}
        </div>
        {presentation.refreshing && (
          <p role="status" className="text-warning">
            {text.rechecking}
          </p>
        )}
        {presentation.state === "failed" && (
          <div>
            {/* 失敗は取り込みの通知が読み上げる。ここでは割り込ませない。 */}
            <Alert variant="destructive" role="note">
              <CircleX aria-hidden="true" />
              <AlertTitle className="break-words">{failureText(presentation)}</AlertTitle>
              <AlertDescription className="flex flex-col items-start gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={retry}
                  disabled={!scan.canStart || scan.running}
                >
                  <RefreshCw />
                  {t.common.retry}
                </Button>
              </AlertDescription>
            </Alert>
          </div>
        )}
        {(issueCount > 0 || issues.items.length > 0) && (
          <ScanIssueList
            list={issues}
            failed={presentation.issues.failed}
            substituted={presentation.issues.substituted}
          />
        )}
      </PageSection>
    </div>
  );
}
