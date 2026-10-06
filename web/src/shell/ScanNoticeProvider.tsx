import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import type { Scan } from "../api/client";
import { inProgress, useScan } from "./ScanProvider";
import {
  dismissedStorageKey,
  emptyScanNoticeSession,
  parseDismissedScanId,
  readDismissedScanId,
  readScanNoticeSession,
  writeDismissedScanId,
  writeScanNoticeSession,
  type CompletionNotice,
  type ScanNoticeSession,
} from "./scanNoticeSession";

const completionNoticeDuration = 8000;

/**
 * persistent は、閉じる button で閉じるか設定へ移るまで残す結果かを返す。一部失敗は、
 * 使えない動画があることを離席していた所有者にも見せるため、失敗と同じく残す
 * （specs/024-import-progress/ui-design.md「Floating Indicator」）。
 */
function persistent(scan: Scan | null | undefined): boolean {
  return scan?.status === "failed" || scan?.status === "partial";
}

export interface ScanNoticeContextValue extends ScanNoticeSession {
  /** 右下の表示を閉じた取り込みの id。 */
  dismissedScanId: number | null;
  /** 開始の途中で閉じ、新しい取り込みの id を待っている。 */
  dismissPending: boolean;
  acknowledgeTerminalScan: () => void;
  /**
   * dismissIndicator は今の取り込みの右下の表示を閉じる。その取り込みが終わるまでも、
   * 終わったあとの結果の通知としても出さない。次の取り込みでは、また出す（issue 830）。
   */
  dismissIndicator: () => void;
  setCompletionNoticePaused: (paused: boolean) => void;
}

const ScanNoticeContext = createContext<ScanNoticeContextValue | null>(null);

export function useScanNotice(): ScanNoticeContextValue {
  const value = useContext(ScanNoticeContext);
  if (value === null)
    throw new Error("useScanNotice must be used inside ScanNoticeProvider");
  return value;
}

export function ScanNoticeProvider({ children }: { children: ReactNode }) {
  const scan = useScan();
  const [session, setSession] = useState<ScanNoticeSession>(readScanNoticeSession);
  const [completionNoticePaused, setCompletionNoticePausedState] = useState(
    () => (session.completionNotice?.pausedRemainingMs ?? null) !== null,
  );
  const sessionRef = useRef(session);
  const scanRef = useRef(scan.scan);
  const [dismissedScanId, setDismissedScanId] = useState(readDismissedScanId);
  const dismissedRef = useRef(dismissedScanId);
  // 開始の途中で閉じたときは、まだ新しい取り込みの id が無い。開始前の id を覚えて待つ。
  const [pendingDismiss, setPendingDismiss] = useState<{
    baseline: number | null;
  } | null>(null);

  const updateSession = useCallback((next: ScanNoticeSession) => {
    sessionRef.current = next;
    setSession(next);
    writeScanNoticeSession(next);
  }, []);

  useEffect(() => {
    scanRef.current = scan.scan;
  }, [scan.scan]);

  const dismiss = useCallback(
    (scanId: number) => {
      dismissedRef.current = scanId;
      setDismissedScanId(scanId);
      writeDismissedScanId(scanId);
      setCompletionNoticePausedState(false);
      const currentSession = sessionRef.current;
      updateSession({
        ...currentSession,
        completionNotice:
          currentSession.completionNotice?.scanId === scanId
            ? null
            : currentSession.completionNotice,
        acknowledgedTerminalScanId: scanId,
      });
    },
    [updateSession],
  );

  // 別のタブで閉じたときも、同じ取り込みは出さない。
  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key !== dismissedStorageKey) return;
      const scanId = parseDismissedScanId(event.newValue);
      dismissedRef.current = scanId;
      setDismissedScanId(scanId);
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  useEffect(() => {
    if (pendingDismiss === null) return;
    const current = scan.scan;
    if (current !== null && current.id !== pendingDismiss.baseline) {
      setPendingDismiss(null);
      dismiss(current.id);
      return;
    }
    // 開始が失敗した。閉じる相手が無いので待つのをやめる。
    if (!scan.starting) setPendingDismiss(null);
  }, [dismiss, pendingDismiss, scan.scan, scan.starting]);

  useEffect(() => {
    const current = scan.scan;
    if (current === null) return;
    // 走査が閉じても準備が残るあいだは status が running のままなので、完了を知らせない。
    if (inProgress(current)) {
      const next: ScanNoticeSession = {
        ...session,
        trackingScanId: current.id,
        completionNotice:
          session.completionNotice?.scanId === current.id
            ? session.completionNotice
            : null,
      };
      if (JSON.stringify(next) !== JSON.stringify(session)) updateSession(next);
      return;
    }
    if (session.trackingScanId !== current.id) return;
    if (session.acknowledgedTerminalScanId === current.id) return;
    // 閉じた取り込みは、終わったあとの結果も通知しない。
    if (dismissedRef.current === current.id) return;
    if (
      session.completionNotice?.scanId === current.id &&
      (persistent(current) || session.completionNotice.pausedRemainingMs !== null)
    ) {
      return;
    }
    if (
      session.completionNotice?.scanId === current.id &&
      session.completionNotice.expiresAt > Date.now()
    ) {
      return;
    }
    if (session.completionNotice?.scanId === current.id) {
      updateSession({
        ...session,
        completionNotice: null,
        acknowledgedTerminalScanId: current.id,
      });
      return;
    }
    const completionNotice: CompletionNotice = {
      scanId: current.id,
      expiresAt: Date.now() + completionNoticeDuration,
      pausedRemainingMs: null,
    };
    updateSession({ ...session, completionNotice });
  }, [scan.scan, session, updateSession]);

  useEffect(() => {
    const notice = session.completionNotice;
    if (notice === null) return;
    if (!scan.loaded) return;
    if (scan.scan?.id === notice.scanId && persistent(scan.scan)) return;
    if (completionNoticePaused || notice.pausedRemainingMs !== null) return;
    const remaining = notice.expiresAt - Date.now();
    const expire = () => {
      const currentSession = sessionRef.current;
      if (currentSession.completionNotice?.scanId !== notice.scanId) return;
      updateSession({
        ...currentSession,
        completionNotice: null,
        acknowledgedTerminalScanId: notice.scanId,
      });
    };
    if (remaining <= 0) {
      expire();
      return;
    }
    const timer = window.setTimeout(expire, remaining);
    return () => window.clearTimeout(timer);
  }, [completionNoticePaused, scan.loaded, scan.scan, session, updateSession]);

  const acknowledgeTerminalScan = useCallback(() => {
    const currentSession = sessionRef.current;
    if (currentSession.completionNotice === null) return;
    setCompletionNoticePausedState(false);
    updateSession({
      ...currentSession,
      completionNotice: null,
      acknowledgedTerminalScanId: currentSession.completionNotice.scanId,
    });
  }, [updateSession]);

  const starting = scan.starting;
  const dismissIndicator = useCallback(() => {
    const current = scanRef.current;
    if (starting) {
      setPendingDismiss({ baseline: current?.id ?? null });
      return;
    }
    if (current !== null) dismiss(current.id);
  }, [dismiss, starting]);

  const setCompletionNoticePaused = useCallback(
    (paused: boolean) => {
      const currentSession = sessionRef.current;
      const notice = currentSession.completionNotice;
      if (
        notice === null ||
        (scanRef.current?.id === notice.scanId && persistent(scanRef.current))
      ) {
        setCompletionNoticePausedState(false);
        return;
      }
      if (paused) {
        setCompletionNoticePausedState(true);
        if (notice.pausedRemainingMs !== null) return;
        updateSession({
          ...currentSession,
          completionNotice: {
            ...notice,
            pausedRemainingMs: Math.max(0, notice.expiresAt - Date.now()),
          },
        });
        return;
      }
      setCompletionNoticePausedState(false);
      if (notice.pausedRemainingMs === null) return;
      updateSession({
        ...currentSession,
        completionNotice: {
          ...notice,
          expiresAt: Date.now() + notice.pausedRemainingMs,
          pausedRemainingMs: null,
        },
      });
    },
    [updateSession],
  );

  const value = useMemo(
    () => ({
      ...session,
      dismissedScanId,
      dismissPending: pendingDismiss !== null,
      acknowledgeTerminalScan,
      dismissIndicator,
      setCompletionNoticePaused,
    }),
    [
      acknowledgeTerminalScan,
      dismissIndicator,
      dismissedScanId,
      pendingDismiss,
      session,
      setCompletionNoticePaused,
    ],
  );
  return (
    <ScanNoticeContext.Provider value={value}>{children}</ScanNoticeContext.Provider>
  );
}

export { emptyScanNoticeSession };
