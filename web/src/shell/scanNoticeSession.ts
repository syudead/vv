export interface CompletionNotice {
  scanId: number;
  expiresAt: number;
  pausedRemainingMs: number | null;
}

export interface ScanNoticeSession {
  trackingScanId: number | null;
  acknowledgedTerminalScanId: number | null;
  completionNotice: CompletionNotice | null;
}

const version = 1;
const storageKey = "vv.scan-notice";

export const emptyScanNoticeSession = (): ScanNoticeSession => ({
  trackingScanId: null,
  acknowledgedTerminalScanId: null,
  completionNotice: null,
});

function validId(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function parse(value: unknown): ScanNoticeSession | null {
  if (typeof value !== "object" || value === null) return null;
  const record = value as Record<string, unknown>;
  if (record.version !== version) return null;
  const trackingScanId = record.trackingScanId;
  const acknowledgedTerminalScanId = record.acknowledgedTerminalScanId;
  const notice = record.completionNotice;
  if (
    !(trackingScanId === null || validId(trackingScanId)) ||
    !(acknowledgedTerminalScanId === null || validId(acknowledgedTerminalScanId))
  ) {
    return null;
  }
  if (notice === null) {
    return { trackingScanId, acknowledgedTerminalScanId, completionNotice: null };
  }
  if (typeof notice !== "object" || notice === null) return null;
  const completionNotice = notice as Record<string, unknown>;
  if (
    !validId(completionNotice.scanId) ||
    typeof completionNotice.expiresAt !== "number" ||
    !(
      completionNotice.pausedRemainingMs === undefined ||
      completionNotice.pausedRemainingMs === null ||
      (typeof completionNotice.pausedRemainingMs === "number" &&
        Number.isFinite(completionNotice.pausedRemainingMs) &&
        completionNotice.pausedRemainingMs >= 0)
    )
  ) {
    return null;
  }
  if (!Number.isFinite(completionNotice.expiresAt) || completionNotice.expiresAt <= 0) {
    return null;
  }
  return {
    trackingScanId,
    acknowledgedTerminalScanId,
    completionNotice: {
      scanId: completionNotice.scanId,
      expiresAt: completionNotice.expiresAt,
      pausedRemainingMs: completionNotice.pausedRemainingMs ?? null,
    },
  };
}

export function readScanNoticeSession(): ScanNoticeSession {
  try {
    const raw = window.sessionStorage.getItem(storageKey);
    if (raw === null) return emptyScanNoticeSession();
    return parse(JSON.parse(raw)) ?? emptyScanNoticeSession();
  } catch {
    return emptyScanNoticeSession();
  }
}

export function writeScanNoticeSession(session: ScanNoticeSession): void {
  try {
    window.sessionStorage.setItem(storageKey, JSON.stringify({ version, ...session }));
  } catch {
    // Notification persistence is best effort; server state remains authoritative.
  }
}

export function clearScanNoticeSession(): void {
  try {
    window.sessionStorage.removeItem(storageKey);
  } catch {
    // Storage may be unavailable in privacy-restricted browsing contexts.
  }
}

/**
 * 右下の表示を閉じた取り込みの id である。同じ取り込みの間は、読み込み直しや別のタブでも
 * 右下に出さない（issue 830）ので、sessionStorage ではなく localStorage に置く。
 */
export const dismissedStorageKey = "vv.scan-indicator-dismissed";

/** parseDismissedScanId は保存した値を読む。形が違えば null を返す。 */
export function parseDismissedScanId(raw: string | null): number | null {
  if (raw === null) return null;
  try {
    const value: unknown = JSON.parse(raw);
    if (typeof value !== "object" || value === null) return null;
    const record = value as Record<string, unknown>;
    if (record.version !== version || !validId(record.scanId)) return null;
    return record.scanId;
  } catch {
    return null;
  }
}

export function readDismissedScanId(): number | null {
  try {
    return parseDismissedScanId(window.localStorage.getItem(dismissedStorageKey));
  } catch {
    return null;
  }
}

export function writeDismissedScanId(scanId: number): void {
  try {
    window.localStorage.setItem(dismissedStorageKey, JSON.stringify({ version, scanId }));
  } catch {
    // Best effort, like the notice session: the indicator only reappears.
  }
}
