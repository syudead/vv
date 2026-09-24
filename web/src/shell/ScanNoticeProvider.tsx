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
  const sessionRef = useRef(session);
  const scanRef = useRef(scan.scan);

  const updateSession = useCallback((next: ScanNoticeSession) => {
    sessionRef.current = next;
    setSession(next);
    writeScanNoticeSession(next);
  }, []);

  useEffect(() => {
    scanRef.current = scan.scan;
  }, [scan.scan]);

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
    if (!scan.loaded) return;
    if (scan.scan?.id === notice.scanId && scan.scan.state === "failed") return;
    if (completionNoticePaused) return;
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
    pausedAt.current = null;
    setCompletionNoticePausedState(false);
    updateSession({
      ...currentSession,
      completionNotice: null,
      acknowledgedTerminalScanId: currentSession.completionNotice.scanId,
    });
  }, [updateSession]);

  const setCompletionNoticePaused = useCallback(
    (paused: boolean) => {
      const currentSession = sessionRef.current;
      const notice = currentSession.completionNotice;
      if (
        notice === null ||
        (scanRef.current?.id === notice.scanId && scanRef.current.state === "failed")
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
        ...currentSession,
        completionNotice: { ...notice, expiresAt: notice.expiresAt + pausedFor },
      });
    },
    [updateSession],
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
