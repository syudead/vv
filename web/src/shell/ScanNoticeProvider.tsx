import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
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
  }, [session, updateSession]);

  const acknowledgeTerminalScan = useCallback(() => {
    if (session.completionNotice === null) return;
    updateSession({
      ...session,
      completionNotice: null,
      acknowledgedTerminalScanId: session.completionNotice.scanId,
    });
  }, [session, updateSession]);

  const value = useMemo(
    () => ({ ...session, acknowledgeTerminalScan }),
    [acknowledgeTerminalScan, session],
  );
  return (
    <ScanNoticeContext.Provider value={value}>{children}</ScanNoticeContext.Provider>
  );
}

export { emptyScanNoticeSession };
