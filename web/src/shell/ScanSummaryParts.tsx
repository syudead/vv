import { AlertTriangle, CheckCircle2, Info, RefreshCw, XCircle } from "lucide-react";

import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import {
  inProgressState,
  type ScanPresentation,
  type ScanPresentationState,
} from "./scanPresentation";

/**
 * ScanStatusIcon は状態の言葉に添えるアイコンである（ui-design.md「Words」）。
 * 取り込み中は回転する。
 */
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
 * ScanIssueCounts は問題の本数である。失敗は `AlertTriangle`、要確認は `Info` を添え、
 * 意味色はアイコンだけに付ける。0 の方は出さない。
 *
 * - `mostSevereOnly`: 失敗があれば失敗だけを、無ければ要確認を出す（右下の本体）。
 * - `compact`: 狭い幅で言葉を隠し、アイコンと数字だけにする。言葉は呼び出し側の名前に残す。
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
        count(failed, t.shell.scan.failedCount(failed), AlertTriangle, "text-danger")}
      {showSubstituted &&
        count(substituted, t.shell.scan.toCheckCount(substituted), Info, "text-warning")}
    </span>
  );
}

/**
 * ScanDetail は今の処理、または完了の時刻の1行である。高さを1行に固定し、今の処理が
 * 変わっても、行が空になっても、配置を動かさない。長いファイル名は末尾を省略し、
 * 省略した全体は `title` で読める。
 */
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
      className={cn("h-5 truncate text-sm leading-5 text-fg-muted", className)}
    >
      {detail?.text}
    </p>
  );
}
