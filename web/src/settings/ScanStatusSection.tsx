import { CircleX, RefreshCw } from "lucide-react";

import { scanErrorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import ScanProgressBar from "../shell/ScanProgressBar";
import { useScan } from "../shell/ScanProvider";
import { ScanDetail, ScanIssueCounts, ScanStatusIcon } from "../shell/ScanSummaryParts";
import { presentScan, type ScanPresentation } from "../shell/scanPresentation";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { PageLinkText } from "./PageLink";
import ScanIssueList from "./ScanIssueList";
import { useScanIssues } from "./useScanIssues";
import { useSectionAnchor } from "./useSectionAnchor";

function stateLabel(presentation: ScanPresentation): UiText | null {
  if (presentation.statusText !== null) return presentation.statusText;
  // 走っている自動の取り込みには状態の言葉が無い（何も出さない）。
  if (presentation.watch) return null;
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
 * ScanStatusSection は設定の「Scan status」の概要である。概要（右下の popover）と同じ要素に、取り込みの開始・分母の
 * 意味・取り込み自体の失敗の理由・問題の一覧だけを足す。取り込みを始める場所はここだけで
 * ある（issue 830）。
 */
export default function ScanStatusSection() {
  const anchor = useSectionAnchor("scan-status");
  const scan = useScan();
  const presentation = presentScan(scan);
  const text = t.settings.scanStatus;
  const issues = useScanIssues(scan.scan, scan.refresh);
  const issueCount = presentation.issues.failed + presentation.issues.substituted;
  // 所有者はいつでもここから取り込みを始める。取り込み中とメディアフォルダが無いときは
  // 押せず、理由を出す（issue 830）。
  const startBlockedReason = scan.running ? (
    text.runningReason
  ) : scan.noMediaFolders ? (
    <PageLinkText sentence={text.needsMediaFolder} page="library" />
  ) : null;

  return (
    <div ref={anchor} id="scan-status">
      <PageSection
        title={text.heading}
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={scan.start}
            disabled={scan.running || !scan.canStart}
            aria-busy={scan.running || undefined}
            aria-describedby={
              startBlockedReason !== null ? "scan-start-blocked" : undefined
            }
          >
            <RefreshCw
              aria-hidden="true"
              className={cn(scan.running && "animate-spin motion-reduce:animate-none")}
            />
            {scan.running ? text.starting : text.start}
          </Button>
        }
        description={
          <span className="flex flex-wrap items-center gap-2">
            {stateLabel(presentation) !== null && (
              <Badge variant="secondary">
                <ScanStatusIcon state={presentation.state} />
                {stateLabel(presentation)}
              </Badge>
            )}
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
        {startBlockedReason !== null && (
          <p id="scan-start-blocked" className="text-xs text-muted-foreground">
            {startBlockedReason}
          </p>
        )}
        {scan.startError !== null && (
          <div>
            {/* 押した操作への応えなので、すぐ読み上げる。 */}
            <Alert variant="destructive" role="alert">
              <CircleX aria-hidden="true" />
              <AlertTitle className="break-words">{scan.startError}</AlertTitle>
            </Alert>
          </div>
        )}
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
