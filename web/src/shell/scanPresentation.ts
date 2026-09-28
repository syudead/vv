import type { Processing, Scan } from "../api/client";
import { scanErrorText, t, type UiText } from "../i18n";
import { processingRemaining, type ScanContextValue } from "./ScanProvider";

export type ScanPresentationState =
  | "not-run"
  | "starting"
  | "unknown-total"
  | "running"
  /** スキャンは終わり、解析・サムネイル・シーク用サムネイル・プレビューの残りがある（またはまだ分からない）。 */
  | "preparing"
  | "done"
  | "partial-failed"
  | "failed"
  | "fetch-failed";

export interface ScanPresentation {
  state: ScanPresentationState;
  scan: Scan | null;
  description: UiText;
  progress: number | null;
  determinate: boolean;
  completed: number;
  total: number | null;
  failed: number;
  startedAt?: string;
  finishedAt?: string;
  error: UiText | null;
  refreshing: boolean;
  /** 段階ごとの残り。未取得なら null。 */
  processing: Processing | null;
  /** 全段階の残りの合計。 */
  remaining: number;
}

function progressFor(scan: Scan): number | null {
  if (scan.total <= 0) return null;
  return Math.min(1, Math.max(0, scan.completed / scan.total));
}

function stateFor(scan: Scan, processing: Processing | null): ScanPresentationState {
  if (scan.state === "running") return scan.total > 0 ? "running" : "unknown-total";
  if (scan.state === "failed") return "failed";
  // 残りをまだ得ていなければ、0 件とみなして完了を示さない。
  if (processing === null || processingRemaining(processing) > 0) return "preparing";
  return scan.failed > 0 ? "partial-failed" : "done";
}

/**
 * describe は状態の一行の説明である。取り込みの失敗は `Scan.error`（自由文）を出さず、
 * `errorCode` と `errorPath` から作る（specs/023-english-i18n/research.md R-6）。
 */
function describe(
  state: ScanPresentationState,
  scan: Scan,
  processing: Processing | null,
  remaining: number,
): UiText {
  const text = t.shell.scan;
  switch (state) {
    case "unknown-total":
      return text.scanningUnknown;
    case "running":
      return text.scanningCount(scan.completed, scan.total);
    case "preparing":
      return processing === null
        ? text.checkingPreparation
        : text.preparingCount(remaining);
    case "partial-failed":
      return text.partialFailed(scan.failed);
    case "failed":
      // コードの無い過去の失敗は、一般的な概要だけにする。
      return scan.errorCode === undefined
        ? scanErrorText(scan)
        : text.failed(scanErrorText(scan));
    default:
      return scan.completed > 0 ? text.lastScanned(scan.completed) : text.noChanges;
  }
}

/** Converts the scan context into the shared state model used by shell views. */
export function presentScan(value: ScanContextValue): ScanPresentation {
  const { scan, processing } = value;
  const remaining = processingRemaining(processing);
  if (value.starting) {
    return {
      state: "starting",
      scan,
      description: t.shell.scan.starting,
      progress: null,
      determinate: false,
      completed: scan?.completed ?? 0,
      total: null,
      failed: scan?.failed ?? 0,
      startedAt: scan?.startedAt,
      finishedAt: scan?.finishedAt,
      error: value.error,
      refreshing: false,
      processing,
      remaining,
    };
  }

  if (scan === null) {
    return {
      state: value.error === null ? "not-run" : "fetch-failed",
      scan: null,
      description: value.error ?? t.shell.scan.notRun,
      progress: null,
      determinate: false,
      completed: 0,
      total: null,
      failed: 0,
      error: value.error,
      refreshing: value.error !== null,
      processing,
      remaining,
    };
  }

  const state = stateFor(scan, processing);
  // 準備の段階は全体の件数が分からないので、割合を出さない。
  const progress = state === "preparing" ? null : progressFor(scan);
  const description = describe(state, scan, processing, remaining);

  return {
    state,
    scan,
    description,
    progress,
    determinate: progress !== null,
    completed: scan.completed,
    total: state === "unknown-total" ? null : scan.total,
    failed: scan.failed,
    startedAt: scan.startedAt,
    finishedAt: scan.finishedAt,
    error: value.error,
    refreshing: value.error !== null,
    processing,
    remaining,
  };
}

export const toScanPresentation = presentScan;
