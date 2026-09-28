import { AlertTriangle, ChevronRight, Info } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { ScanIssue } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { ScanIssueCounts } from "../shell/ScanSummaryParts";
import Button from "../ui/Button";
import type { ScanIssueList as ScanIssueListState } from "./useScanIssues";

/** issueLocation は「登録フォルダの表示名 / 相対パス」である。 */
function issueLocation(issue: ScanIssue): string {
  const parts: string[] = [];
  if (issue.folder.rootName !== undefined) parts.push(issue.folder.rootName);
  if (issue.folder.path !== "") parts.push(issue.folder.path);
  return t.shell.scan.location(parts);
}

/** issueWords は行の影響と理由、ほかの種類の影響である（ui-design.md「Words」）。 */
export function issueWords(issue: ScanIssue): {
  impact: UiText;
  impactAndReason: UiText;
  also: UiText | null;
} {
  const text = t.settings.scanStatus.issues;
  const [first, ...rest] = issue.kinds;
  const main = first === undefined ? null : text.kinds[first];
  const impact = main?.impact ?? text.kinds.register_failed.impact;
  return {
    impact,
    impactAndReason:
      main === null ? impact : text.impactAndReason(main.impact, main.reason),
    also:
      rest.length === 0 ? null : text.also(rest.map((kind) => text.kinds[kind].impact)),
  };
}

function IssueRow({ issue }: { issue: ScanIssue }) {
  const text = t.settings.scanStatus.issues;
  const failed = issue.severity === "failed";
  const marker = failed ? text.failed : text.check;
  const Icon = failed ? AlertTriangle : Info;
  const location = issueLocation(issue);
  const words = issueWords(issue);
  const content = (
    <>
      <span
        className={cn(
          "col-start-1 row-start-1 inline-flex items-center gap-1 text-xs font-medium leading-5 whitespace-nowrap md:row-span-4",
          failed ? "text-danger" : "text-warning",
        )}
      >
        <Icon aria-hidden className="size-4 shrink-0" />
        {marker}
      </span>
      <span
        title={issue.fileName}
        className="col-start-2 row-start-1 min-w-0 truncate text-sm font-medium leading-5 text-fg"
      >
        {issue.fileName}
      </span>
      <span
        title={location}
        dir="rtl"
        className="col-span-2 col-start-1 truncate text-left text-xs text-fg-muted md:col-span-1 md:col-start-2"
      >
        <bdi dir="ltr">{location}</bdi>
      </span>
      <span className="col-span-2 col-start-1 text-sm break-words text-fg md:col-span-1 md:col-start-2">
        {words.impactAndReason}
      </span>
      {words.also !== null && (
        <span className="col-span-2 col-start-1 text-xs break-words text-fg-muted md:col-span-1 md:col-start-2">
          {words.also}
        </span>
      )}
    </>
  );
  const grid =
    "grid grid-cols-[auto_minmax(0,1fr)_auto] content-start gap-x-2 gap-y-0.5 px-3 py-2.5 md:grid-cols-[5.5rem_minmax(0,1fr)_auto]";

  if (issue.videoId === undefined) {
    return (
      <li data-testid="scan-issue" className={grid}>
        {content}
      </li>
    );
  }
  return (
    <li data-testid="scan-issue">
      <Link
        to={`/videos/${String(issue.videoId)}`}
        aria-label={text.openVideo(issue.fileName, marker, words.impact)}
        className={cn(
          grid,
          "transition-colors hover:bg-hover-wash focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-link",
        )}
      >
        {content}
        <ChevronRight
          aria-hidden
          className="col-start-3 row-span-4 row-start-1 size-4 self-center text-fg-muted"
        />
      </Link>
    </li>
  );
}

/**
 * ScanIssueList は設定の「Scan status」の問題の一覧である（specs/024-import-progress/ui-design.md
 * 「Issue List」）。高さに上限のある縦スクロールの領域に置き、何件あっても下の section を
 * 押し流さない。
 */
export default function ScanIssueList({
  list,
  failed,
  substituted,
}: {
  list: ScanIssueListState;
  failed: number;
  substituted: number;
}) {
  const text = t.settings.scanStatus.issues;
  const region = useRef<HTMLDivElement>(null);

  // 新しい取り込みの一覧に入れ替わったら先頭へ戻す。同じ取り込みの読み直しでは保つ。
  useEffect(() => {
    if (region.current !== null) region.current.scrollTop = 0;
  }, [list.scanId]);

  return (
    <div className="mt-4 border-t border-border pt-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h3 id="scan-issues-heading" className="text-sm font-semibold">
          {text.heading}
        </h3>
        <ScanIssueCounts
          failed={failed}
          substituted={substituted}
          className="text-xs text-fg-muted"
        />
      </div>
      {list.error !== null && (
        <p role="status" className="mt-2 text-sm text-warning">
          {text.loadFailed(list.error)}
        </p>
      )}
      <div
        ref={region}
        role="region"
        aria-labelledby="scan-issues-heading"
        tabIndex={0}
        data-testid="scan-issue-region"
        className="mt-2 max-h-[50vh] overflow-y-auto rounded-md border border-border focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-link"
      >
        <ul className="divide-y divide-border">
          {list.items.map((issue) => (
            <IssueRow
              key={`${String(issue.folder.rootId)}/${issue.folder.path}/${issue.fileName}`}
              issue={issue}
            />
          ))}
        </ul>
        {list.nextCursor !== undefined && (
          <div className="border-t border-border p-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={list.loadMore}
              aria-busy={list.loadingMore}
            >
              {list.loadingMore ? text.loadingMore : text.showMore}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
