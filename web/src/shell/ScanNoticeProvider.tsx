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

import { useScan } from "./ScanProvider";
import {
  emptyScanNoticeSession,
  readScanNoticeSession,
  writeScanNoticeSession,
  type CompletionNotice,
  type ScanNoticeSession,
} from "./scanNoticeSession";

const completionNoticeDuration = 8000;

export interface ScanNoticeContextValue extends ScanNoticeSession {
  acknowledgeTerminalScan: () => void;
  setCompletionNoticePaused: (paused: boolean) => void;
}

const ScanNoticeContext = createContext<ScanNoticeContextValue | null>(null);

export function useScanNotice(): ScanNoticeContextValue {
  const value = useContext(ScanNoticeContext);
  if (value === null) throw new Error("useScanNotice は ScanNoticeProvider の中で使う");
  return value;
}

export function ScanNoticeProvider({ children }: { children: ReactNode }) {
  const scan = useScan();
  const [session, setSession] = useState<ScanNoticeSession>(readScanNoticeSession);
  const [completionNoticePaused, setCompletionNoticePausedState] = useState(false);
  const pausedAt = useRef<number | null>(null);

  const updateSession = useCallback((next: ScanNoticeSession) => {
    setSession(next);
    writeScanNoticeSession(next);
  }, []);

  useEffect(() => {
    const current = scan.scan;
    if (current === null) return;
    if (current.state === "running") {
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
    if (session.completionNotice?.scanId === current.id && current.state === "failed") {
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
    };
    updateSession({ ...session, completionNotice });
  }, [scan.scan, session, updateSession]);

  useEffect(() => {
    const notice = session.completionNotice;
    if (notice === null) return;
    if (scan.scan?.id === notice.scanId && scan.scan.state === "failed") return;
    if (completionNoticePaused) return;
    const remaining = notice.expiresAt - Date.now();
    if (remaining <= 0) {
      updateSession({
        ...session,
        completionNotice: null,
        acknowledgedTerminalScanId: notice.scanId,
      });
      return;
    }
    const timer = window.setTimeout(() => {
      updateSession({
        ...session,
        completionNotice: null,
        acknowledgedTerminalScanId: notice.scanId,
      });
    }, remaining);
    return () => window.clearTimeout(timer);
  }, [completionNoticePaused, scan.scan, session, updateSession]);

  const acknowledgeTerminalScan = useCallback(() => {
    if (session.completionNotice === null) return;
    pausedAt.current = null;
    setCompletionNoticePausedState(false);
    updateSession({
      ...session,
      completionNotice: null,
      acknowledgedTerminalScanId: session.completionNotice.scanId,
    });
  }, [session, updateSession]);

  const setCompletionNoticePaused = useCallback(
    (paused: boolean) => {
      const notice = session.completionNotice;
      if (
        notice === null ||
        (scan.scan?.id === notice.scanId && scan.scan.state === "failed")
      ) {
        pausedAt.current = null;
        setCompletionNoticePausedState(false);
        return;
      }
      if (paused) {
        pausedAt.current ??= Date.now();
        setCompletionNoticePausedState(true);
        return;
      }
      if (pausedAt.current === null) return;
      const pausedFor = Date.now() - pausedAt.current;
      pausedAt.current = null;
      setCompletionNoticePausedState(false);
      updateSession({
        ...session,
        completionNotice: { ...notice, expiresAt: notice.expiresAt + pausedFor },
      });
    },
    [scan.scan, session, updateSession],
  );

  const value = useMemo(
    () => ({ ...session, acknowledgeTerminalScan, setCompletionNoticePaused }),
    [acknowledgeTerminalScan, session, setCompletionNoticePaused],
  );
  return (
    <ScanNoticeContext.Provider value={value}>{children}</ScanNoticeContext.Provider>
  );
}

export { emptyScanNoticeSession };
