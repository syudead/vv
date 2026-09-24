import type { Processing, Scan } from "../api/client";
import { processingRemaining, type ScanContextValue } from "./ScanProvider";

export type ScanPresentationState =
  | "not-run"
  | "starting"
  | "unknown-total"
  | "running"
  /** スキャンは終わり、解析・サムネイル・プレビューの残りがある。 */
  | "preparing"
  | "done"
  | "partial-failed"
  | "failed"
  | "fetch-failed";

export interface ScanPresentation {
  state: ScanPresentationState;
  scan: Scan | null;
  description: string;
  progress: number | null;
  determinate: boolean;
  completed: number;
  total: number | null;
  failed: number;
  startedAt?: string;
  finishedAt?: string;
  error: string | null;
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

function stateFor(scan: Scan, remaining: number): ScanPresentationState {
  if (scan.state === "running") return scan.total > 0 ? "running" : "unknown-total";
  if (scan.state === "failed") return "failed";
  if (remaining > 0) return "preparing";
  return scan.failed > 0 ? "partial-failed" : "done";
}

/** Converts the scan context into the shared state model used by shell views. */
export function presentScan(value: ScanContextValue): ScanPresentation {
  const { scan, processing } = value;
  const remaining = processingRemaining(processing);
  if (value.starting) {
    return {
      state: "starting",
      scan,
      description: "取り込みを開始しています…",
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
      description: value.error ?? "まだ取り込んでいません",
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

  const state = stateFor(scan, remaining);
  // 準備の段階は全体の件数が分からないので、割合を出さない。
  const progress = state === "preparing" ? null : progressFor(scan);
  const description =
    state === "unknown-total"
      ? "取り込み中…"
      : state === "running"
        ? `取り込み中 ${String(scan.completed)} / ${String(scan.total)}`
        : state === "preparing"
          ? `取り込んだ動画を準備中（残り ${String(remaining)} 件）`
          : state === "partial-failed"
            ? `一部失敗（${String(scan.failed)} 件）`
            : state === "failed"
              ? `取り込みに失敗しました: ${scan.error ?? "理由は記録されていません"}`
              : scan.completed > 0
                ? `前回 ${String(scan.completed)} 件を取り込みました`
                : "前回の取り込みで変化はありませんでした";

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
