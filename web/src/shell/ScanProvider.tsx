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

import {
  getCurrentScan,
  isAborted,
  listMediaFolders,
  RequestFailed,
  startRescan,
  startScan,
  type Scan,
  type ScanActivity,
  type StartRescanRequest,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { useAudience } from "../auth/audience";
import { errorText, t, type UiText } from "../i18n";

export interface ScanContextValue {
  scan: Scan | null;
  loaded: boolean;
  /** Retain the previous activity across temporary gaps within the same running scan. */
  activity: ScanActivity | null;
  error: UiText | null;
  startError: UiText | null;
  starting: boolean;
  running: boolean;
  canStart: boolean;
  /** False while the folder count remains unknown. */
  noMediaFolders: boolean;
  start: () => void;
  /**
   * Start a force rescan. Resolves to the failure for the caller to show beside its control, or
   * null once started; it leaves startError alone. When the failure may hide a start whose response
   * was lost, onRecovered runs once a later read or event shows the new scan.
   */
  rescan: (
    request: StartRescanRequest,
    onRecovered?: () => void,
  ) => Promise<UiText | null>;
  refresh: () => void;
  setFolderCount: (count: number) => void;
  /** Exclude scans already finished on initial load so lists do not refresh unexpectedly. */
  finished: Scan | null;
  /**
   * Track automatic completion separately for in-place list refreshes, using status after
   * preparation finishes.
   */
  watchFinished: Scan | null;
}

/** Lists use this stable control context to avoid rerendering on per-file scan events. */
export type ScanControlsValue = Pick<
  ScanContextValue,
  | "running"
  | "canStart"
  | "noMediaFolders"
  | "start"
  | "rescan"
  | "refresh"
  | "setFolderCount"
  | "finished"
  | "watchFinished"
>;

/** Coalesce only recent in-flight reads; abort older hung reads and retry. */
const dedupeLoadMs = 1000;

const ScanContext = createContext<ScanContextValue | null>(null);
const ScanControlsContext = createContext<ScanControlsValue | null>(null);

export function useScan(): ScanContextValue {
  const value = useContext(ScanContext);
  if (value === null) {
    throw new Error("useScan must be used inside ScanProvider");
  }
  return value;
}

export function useScanControls(): ScanControlsValue {
  const value = useScanControlsIfProvided();
  if (value === null) {
    throw new Error("useScanControls must be used inside ScanProvider");
  }
  return value;
}

/** For an optional control on a screen that can also render without ScanProvider. */
export function useScanControlsIfProvided(): ScanControlsValue | null {
  return useContext(ScanControlsContext);
}

/** A 4xx answer is the server's refusal; anything else may follow a start whose response was lost. */
function mayHaveStarted(failure: unknown): boolean {
  return !(failure instanceof RequestFailed && failure.status < 500);
}

/** Running watcher scans neither display progress nor block manual-start availability. */
export function isWatchScan(scan: Scan | null): boolean {
  return scan?.origin === "watch";
}

export function inProgress(scan: Scan): boolean {
  return scan.status === "finding" || scan.status === "running";
}

/** Retain missing activity only within the same unfinished scan. */
export function nextActivity(
  previous: { scanId: number; activity: ScanActivity } | null,
  current: Scan | null,
): { scanId: number; activity: ScanActivity } | null {
  if (current === null || !inProgress(current)) return null;
  if (current.activity !== undefined) {
    return { scanId: current.id, activity: current.activity };
  }
  return previous?.scanId === current.id ? previous : null;
}

/**
 * Owners share one event-driven scan state, refreshed on reconnect, focus or explicit request.
 * Guests neither fetch nor subscribe; their controls are no-ops.
 */
export function ScanProvider({ children }: { children: ReactNode }) {
  const owner = useAudience() === "owner";
  const [scan, setScan] = useState<Scan | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [activity, setActivity] = useState<{
    scanId: number;
    activity: ScanActivity;
  } | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [startError, setStartError] = useState<UiText | null>(null);
  const [starting, setStarting] = useState(false);
  const [finished, setFinished] = useState<Scan | null>(null);
  const [watchFinished, setWatchFinished] = useState<Scan | null>(null);
  const observedRunningWatchId = useRef<number | null>(null);
  const handledWatchId = useRef<number | null>(null);
  const [folderCount, setFolderCount] = useState<number | null>(null);
  const folderCountRevision = useRef(0);
  const requestedScanId = useRef<number | null>(null);
  const observedRunningScanId = useRef<number | null>(null);
  const recoveryBaselineScanId = useRef<number | null | undefined>(undefined);
  const onRescanRecovered = useRef<(() => void) | null>(null);
  const lastSeenScanId = useRef<number | null | undefined>(undefined);
  // Events can be newer than pending reads; discard a read if an event arrived after it started.
  const scanRevision = useRef(0);

  const updateFolderCount = useCallback((count: number) => {
    folderCountRevision.current += 1;
    setFolderCount(count);
  }, []);

  const apply = useCallback((current: Scan | null) => {
    const previousScanId = lastSeenScanId.current;
    lastSeenScanId.current = current?.id ?? null;
    setScan(current);
    setActivity((previous) => nextActivity(previous, current));
    setLoaded(true);
    setLoadError(null);
    if (current === null) return;

    if (current.origin === "watch") {
      // Automatic completion refreshes lists only after preparation and only for scans observed after initial load.
      if (inProgress(current)) {
        observedRunningWatchId.current = current.id;
      } else if (
        handledWatchId.current !== current.id &&
        (observedRunningWatchId.current === current.id ||
          (previousScanId !== undefined && current.id !== previousScanId))
      ) {
        handledWatchId.current = current.id;
        setWatchFinished(current);
      }
      return;
    }

    if (
      previousScanId !== undefined &&
      current.id !== previousScanId &&
      current.state !== "running"
    ) {
      setFinished(current);
    }

    const recoveryBaseline = recoveryBaselineScanId.current;
    const recovered =
      recoveryBaseline !== undefined &&
      (current.state === "running" || current.id !== recoveryBaseline);
    if (recovered) {
      recoveryBaselineScanId.current = undefined;
      setStartError(null);
      const notify = onRescanRecovered.current;
      onRescanRecovered.current = null;
      notify?.();
    }

    if (current.state === "running") {
      observedRunningScanId.current = current.id;
      return;
    }
    const completedObservedScan = observedRunningScanId.current === current.id;
    const completedRequestedScan = requestedScanId.current === current.id;
    if (completedObservedScan || completedRequestedScan || recovered) {
      setFinished(current);
    }
    if (completedObservedScan) observedRunningScanId.current = null;
    if (completedRequestedScan) requestedScanId.current = null;
  }, []);

  // Track fetch failure to decide whether initial connection needs another read.
  const loadFailed = useRef(false);

  const loadFolders = useCallback(async (signal: AbortSignal) => {
    const revision = folderCountRevision.current;
    try {
      const folders = await listMediaFolders(signal);
      if (revision === folderCountRevision.current) setFolderCount(folders.length);
    } catch (failure) {
      if (isAborted(failure) || revision !== folderCountRevision.current) return;
      loadFailed.current = true;
      setFolderCount(null);
    }
  }, []);

  const inFlight = useRef<AbortController | null>(null);
  const loadScan = useCallback(() => {
    inFlight.current?.abort();
    const controller = new AbortController();
    inFlight.current = controller;
    scanRevision.current += 1;
    const scanAt = scanRevision.current;

    void (async () => {
      try {
        const current = await getCurrentScan(controller.signal);
        if (scanAt === scanRevision.current) apply(current);
      } catch (failure) {
        if (scanAt !== scanRevision.current || isAborted(failure)) return;
        // Keep the last known state on failure; retry on reconnect or window focus.
        loadFailed.current = true;
        setLoadError(errorText(failure));
      } finally {
        if (inFlight.current === controller) inFlight.current = null;
      }
    })();
  }, [apply]);

  const foldersInFlight = useRef<AbortController | null>(null);
  const loadStartedAt = useRef(0);
  const load = useCallback(() => {
    loadStartedAt.current = Date.now();
    foldersInFlight.current?.abort();
    const controller = new AbortController();
    foldersInFlight.current = controller;
    loadFailed.current = false;
    void loadFolders(controller.signal).finally(() => {
      if (foldersInFlight.current === controller) foldersInFlight.current = null;
    });
    loadScan();
  }, [loadFolders, loadScan]);

  /**
   * Coalesce recent reads from child and parent mount effects; abort older hung requests before
   * retrying.
   */
  const loadUnlessLoading = useCallback(() => {
    // Aborted reads are no longer pending, allowing StrictMode and audience-change effects to restart them.
    const pending = (controller: AbortController | null) =>
      controller !== null && !controller.signal.aborted;
    const loading = pending(inFlight.current) || pending(foldersInFlight.current);
    if (loading && Date.now() - loadStartedAt.current < dedupeLoadMs) return;
    load();
  }, [load]);

  useEffect(() => {
    if (!owner) return;
    // Subscribe before reading to avoid missing changes after the snapshot.
    const unsubscribe = subscribeServerEvents({
      scan: (next) => {
        scanRevision.current += 1;
        apply(next);
      },
      // Initial connection already sends current scan state; refetch only after a prior failure. Reconnect also refreshes folder counts missed while disconnected.
      open: (reconnected) => {
        if (reconnected || loadFailed.current) load();
      },
    });
    loadUnlessLoading();
    window.addEventListener("focus", loadUnlessLoading);
    return () => {
      unsubscribe();
      window.removeEventListener("focus", loadUnlessLoading);
      inFlight.current?.abort();
      foldersInFlight.current?.abort();
    };
  }, [apply, load, loadUnlessLoading, owner]);

  const refresh = useCallback(() => {
    if (owner) loadUnlessLoading();
  }, [loadUnlessLoading, owner]);

  /**
   * A null request starts a manual scan, whose failure stays in startError for Settings. A force
   * rescan skips the folder-count guard so the server's reason reaches the caller instead of a
   * silent no-op.
   */
  const begin = useCallback(
    async (
      request: StartRescanRequest | null,
      onRecovered?: () => void,
    ): Promise<UiText | null> => {
      if (!owner) return null;
      if (request === null && (folderCount === null || folderCount === 0)) return null;
      const baselineScanId = scan?.id ?? null;
      setStarting(true);
      setStartError(null);
      recoveryBaselineScanId.current = undefined;
      onRescanRecovered.current = null;
      // Fast scans may emit events before start returns; never replace newer event state with the response.
      const requestedAt = scanRevision.current;
      let failed: UiText | null = null;
      try {
        const started = await (request === null ? startScan() : startRescan(request));
        if (requestedAt === scanRevision.current) {
          scanRevision.current += 1;
          requestedScanId.current = started.id;
          if (started.state === "running") {
            observedRunningScanId.current = started.id;
          }
          apply(started);
        }
      } catch (failure) {
        if (
          failure instanceof RequestFailed &&
          failure.code === "media_folders_not_configured"
        ) {
          updateFolderCount(0);
        }
        failed = errorText(failure);
        // A lost response may hide a successful start; clear the error when events or reads discover the scan.
        if (request === null) {
          setStartError(t.shell.scan.startFailed(failed));
          recoveryBaselineScanId.current = baselineScanId;
        } else if (mayHaveStarted(failure)) {
          recoveryBaselineScanId.current = baselineScanId;
          onRescanRecovered.current = onRecovered ?? null;
        }
      } finally {
        setStarting(false);
      }
      // Refresh once immediately because a small scan may already have finished.
      loadScan();
      return failed;
    },
    [apply, folderCount, loadScan, owner, scan?.id, updateFolderCount],
  );

  const start = useCallback(() => void begin(null), [begin]);
  const rescan = useCallback(
    (request: StartRescanRequest, onRecovered?: () => void) =>
      begin(request, onRecovered),
    [begin],
  );

  const error = startError ?? loadError;

  const value = useMemo<ScanContextValue>(
    () => ({
      scan,
      loaded,
      activity: activity?.activity ?? null,
      error,
      startError,
      starting,
      running: starting || (scan?.state === "running" && !isWatchScan(scan)),
      canStart: folderCount !== null && folderCount > 0,
      noMediaFolders: folderCount === 0,
      start,
      rescan,
      refresh,
      setFolderCount: updateFolderCount,
      finished,
      watchFinished,
    }),
    [
      activity,
      error,
      finished,
      watchFinished,
      folderCount,
      loaded,
      refresh,
      rescan,
      scan,
      start,
      startError,
      starting,
      updateFolderCount,
    ],
  );

  const running = starting || (scan?.state === "running" && !isWatchScan(scan));
  const canStart = folderCount !== null && folderCount > 0;
  const noMediaFolders = folderCount === 0;
  const controls = useMemo<ScanControlsValue>(
    () => ({
      running,
      canStart,
      noMediaFolders,
      start,
      rescan,
      refresh,
      setFolderCount: updateFolderCount,
      finished,
      watchFinished,
    }),
    [
      canStart,
      finished,
      noMediaFolders,
      refresh,
      rescan,
      running,
      start,
      updateFolderCount,
      watchFinished,
    ],
  );

  return (
    <ScanControlsContext.Provider value={controls}>
      <ScanContext.Provider value={value}>{children}</ScanContext.Provider>
    </ScanControlsContext.Provider>
  );
}
