import { ChevronRight, Info, TriangleAlert } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { ScanIssue } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { ScanIssueCounts } from "../shell/ScanSummaryParts";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import type { ScanIssueList as ScanIssueListState } from "./useScanIssues";

function issueLocation(issue: ScanIssue): string {
  const parts: string[] = [];
  if (issue.folder.rootName !== undefined) parts.push(issue.folder.rootName);
  if (issue.folder.path !== "") parts.push(issue.folder.path);
  return t.shell.scan.location(parts);
}

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
  const Icon = failed ? TriangleAlert : Info;
  const location = issueLocation(issue);
  const words = issueWords(issue);
  const content = (
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      <div className="flex min-w-0 items-center gap-2">
        <Badge variant={failed ? "destructive" : "warning"}>
          <Icon aria-hidden />
          {marker}
        </Badge>
        <span title={issue.fileName} className="min-w-0 truncate font-medium">
          {issue.fileName}
        </span>
      </div>
      <span
        title={location}
        dir="rtl"
        className="truncate text-left text-xs text-muted-foreground"
      >
        <bdi dir="ltr">{location}</bdi>
      </span>
      <span className="break-words">{words.impactAndReason}</span>
      {words.also !== null && (
        <span className="text-xs break-words text-muted-foreground">{words.also}</span>
      )}
    </div>
  );
  const row = "flex min-w-0 items-center gap-2 px-3 py-2";

  if (issue.videoId === undefined) {
    return (
      <li data-testid="scan-issue" className={row}>
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
          row,
          "transition-colors hover:bg-accent focus-visible:-outline-offset-2",
        )}
      >
        {content}
        <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
      </Link>
    </li>
  );
}

/** Bound the issue list's height so large results cannot push subsequent sections away. */
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

  // Reset scroll only for a new scan, retaining position on same-scan refresh.
  useEffect(() => {
    if (region.current !== null) region.current.scrollTop = 0;
  }, [list.scanId]);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h3 id="scan-issues-heading" className="font-semibold">
          {text.heading}
        </h3>
        <ScanIssueCounts
          failed={failed}
          substituted={substituted}
          className="text-xs text-muted-foreground"
        />
      </div>
      {/* Announce only completion, partial failure and failure. */}
      {list.error !== null && (
        <p className="text-warning">{text.loadFailed(list.error)}</p>
      )}
      <div
        ref={region}
        role="region"
        aria-labelledby="scan-issues-heading"
        tabIndex={0}
        data-testid="scan-issue-region"
        className="max-h-issue-list overflow-y-auto rounded-md border border-border"
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
