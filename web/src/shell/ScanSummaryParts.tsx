import { AlertTriangle, CheckCircle2, Info, RefreshCw, XCircle } from "lucide-react";

import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import {
  inProgressState,
  type ScanPresentation,
  type ScanPresentationState,
} from "./scanPresentation";

export function ScanStatusIcon({
  state,
  className,
}: {
  state: ScanPresentationState;
  className?: string;
}) {
  const classes = cn("size-4 shrink-0", className);
  if (state === "done") return <CheckCircle2 aria-hidden className={classes} />;
  if (state === "partial") return <AlertTriangle aria-hidden className={classes} />;
  if (state === "failed") return <XCircle aria-hidden className={classes} />;
  return (
    <RefreshCw
      aria-hidden
      className={cn(
        classes,
        inProgressState(state) && "animate-spin motion-reduce:animate-none",
      )}
    />
  );
}

/**
 * mostSevereOnly prefers failures over review-needed counts; compact hides visible labels while
 * callers retain accessible names.
 */
export function ScanIssueCounts({
  failed,
  substituted,
  mostSevereOnly = false,
  compact = false,
  className,
}: {
  failed: number;
  substituted: number;
  mostSevereOnly?: boolean;
  compact?: boolean;
  className?: string;
}) {
  const showSubstituted = substituted > 0 && !(mostSevereOnly && failed > 0);
  if (failed === 0 && !showSubstituted) return null;
  const count = (value: number, text: UiText, Icon: typeof Info, tone: string) => (
    <span className="inline-flex items-center gap-1 whitespace-nowrap">
      <Icon aria-hidden className={cn("size-4 shrink-0", tone)} />
      {compact ? (
        <>
          <span className="hidden sm:inline">{text}</span>
          <span className="sm:hidden">{formatNumber(value)}</span>
        </>
      ) : (
        text
      )}
    </span>
  );
  return (
    <span
      className={cn("inline-flex flex-wrap items-center gap-x-2 tabular-nums", className)}
    >
      {failed > 0 &&
        count(
          failed,
          t.shell.scan.failedCount(failed),
          AlertTriangle,
          "text-destructive",
        )}
      {showSubstituted &&
        count(substituted, t.shell.scan.toCheckCount(substituted), Info, "text-warning")}
    </span>
  );
}

/** Reserve one line even when activity disappears to prevent layout shifts. */
export function ScanDetail({
  presentation,
  className,
}: {
  presentation: ScanPresentation;
  className?: string;
}) {
  const detail = presentation.detail;
  return (
    <p
      data-testid="scan-detail"
      title={detail?.title}
      className={cn("h-5 truncate text-sm leading-5 text-muted-foreground", className)}
    >
      {detail?.text}
    </p>
  );
}
