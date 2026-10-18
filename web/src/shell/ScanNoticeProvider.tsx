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

import { isAborted } from "../api/client";
import { inProgress, useScan } from "./ScanProvider";
import {
  hasNewFailure,
  previousFailureKeys,
  readFailureKeys,
  rememberFailureKeys,
} from "./scanIssueMemory";
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

export interface ScanNoticeContextValue extends ScanNoticeSession {
  dismissedScanId: number | null;
  dismissPending: boolean;
  acknowledgeTerminalScan: () => void;
  /** Dismiss for the entire scan, including its terminal notification; the next scan may show again. */
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
  // A pending start has no new scan ID yet; remember the prior ID and wait.
  // A lost start response may still have started the scan; keep waiting for events or reads until another start begins.
  const [pendingDismiss, setPendingDismiss] = useState<{
    baseline: number | null;
    settled: boolean;
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

  // Honor dismissal in other tabs too.
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
    if (!scan.starting && !pendingDismiss.settled) {
      setPendingDismiss({ ...pendingDismiss, settled: true });
    } else if (scan.starting && pendingDismiss.settled) {
      setPendingDismiss(null);
    }
  }, [dismiss, pendingDismiss, scan.scan, scan.starting]);

  useEffect(() => {
    const current = scan.scan;
    if (current === null) return;
    // Running automatic scans stay silent; the terminal effect decides notifications.
    if (current.origin === "watch") return;
    // Scanning may finish while preparation keeps status running; do not announce completion yet.
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
    // Dismissal also suppresses terminal notifications.
    if (dismissedRef.current === current.id) return;
    if (
      session.completionNotice?.scanId === current.id &&
      session.completionNotice.pausedRemainingMs !== null
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

  // Remember manual-scan failures so later automatic scans do not announce carried-over failures as new.
  const manualFinished = scan.finished;
  useEffect(() => {
    if (manualFinished === null) return;
    if (manualFinished.issues.failed === 0) {
      rememberFailureKeys(manualFinished.id, new Set());
      return;
    }
    const abort = new AbortController();
    readFailureKeys(manualFinished.id, abort.signal)
      .then((keys) => {
        if (keys !== null) rememberFailureKeys(manualFinished.id, keys);
      })
      .catch(() => undefined);
    return () => abort.abort();
  }, [manualFinished]);

  // Notify automatic failure or partial completion only for new path/type failures; without a previous snapshot, treat failures as new.
  const watchFinished = scan.watchFinished;
  useEffect(() => {
    if (watchFinished === null) return;
    if (watchFinished.status !== "partial" && watchFinished.status !== "failed") {
      if (watchFinished.issues.failed === 0) {
        rememberFailureKeys(watchFinished.id, new Set());
      }
      return;
    }
    const abort = new AbortController();
    const show = () => {
      if (abort.signal.aborted || dismissedRef.current === watchFinished.id) return;
      if (scanRef.current?.id !== watchFinished.id) return;
      const currentSession = sessionRef.current;
      updateSession({
        ...currentSession,
        completionNotice: {
          scanId: watchFinished.id,
          expiresAt: Date.now() + completionNoticeDuration,
          pausedRemainingMs: null,
        },
      });
    };
    if (watchFinished.status === "failed") {
      // Remember failures even from failed scans for later comparison; notify without waiting for that read.
      show();
      readFailureKeys(watchFinished.id, abort.signal)
        .then((keys) => {
          if (!abort.signal.aborted && keys !== null) {
            rememberFailureKeys(watchFinished.id, keys);
          }
        })
        .catch(() => undefined);
      return () => abort.abort();
    }
    readFailureKeys(watchFinished.id, abort.signal)
      .then((keys) => {
        if (abort.signal.aborted || keys === null) return;
        const isNew = hasNewFailure(keys, previousFailureKeys());
        rememberFailureKeys(watchFinished.id, keys);
        if (isNew) show();
      })
      .catch((failure: unknown) => {
        // On read failure, err toward reporting a possible new failure.
        if (!isAborted(failure)) show();
      });
    return () => abort.abort();
  }, [updateSession, watchFinished]);

  useEffect(() => {
    const notice = session.completionNotice;
    if (notice === null) return;
    if (!scan.loaded) return;
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
      setPendingDismiss({ baseline: current?.id ?? null, settled: false });
      return;
    }
    if (current !== null) dismiss(current.id);
  }, [dismiss, starting]);

  const setCompletionNoticePaused = useCallback(
    (paused: boolean) => {
      const currentSession = sessionRef.current;
      const notice = currentSession.completionNotice;
      if (notice === null) {
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
