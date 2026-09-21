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
  errorMessage,
  getCurrentScan,
  isAborted,
  listMediaFolders,
  RequestFailed,
  startScan,
  type Scan,
} from "../api/client";

const pollInterval = 2000;
const recoveryPollLimit = 15;

export interface ScanContextValue {
  scan: Scan | null;
  /** 状態取得や開始の失敗。 */
  error: string | null;
  starting: boolean;
  running: boolean;
  canStart: boolean;
  start: () => void;
  refresh: () => void;
  setFolderCount: (count: number) => void;
  /**
   * 「終わったのを見た」スキャン。初回表示で既に終わっていたものは含めない
   * （利用者が待っていたものではないため、一覧を勝手に入れ替えない）。
   */
  finished: Scan | null;
}

const ScanContext = createContext<ScanContextValue | null>(null);

export function useScan(): ScanContextValue {
  const value = useContext(ScanContext);
  if (value === null) {
    throw new Error("useScan は ScanProvider の中で使う");
  }
  return value;
}

/**
 * ScanProvider は取り込み状態をひとつ持ち、実行中は 2 秒ごとに追いかける。
 * トップバーのボタンとライブラリの再読込が同じ状態を見る。
 */
export function ScanProvider({ children }: { children: ReactNode }) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [pollError, setPollError] = useState<string | null>(null);
  const [startError, setStartError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [finished, setFinished] = useState<Scan | null>(null);
  const [watch, setWatch] = useState(0);
  const [folderWatch, setFolderWatch] = useState(0);
  const [folderCount, setFolderCount] = useState<number | null>(null);
  const folderCountRevision = useRef(0);
  const requestedScanId = useRef<number | null>(null);
  const observedRunningScanId = useRef<number | null>(null);
  const recoveryBaselineScanId = useRef<number | null | undefined>(undefined);
  const recoveryPollsLeft = useRef(0);
  const lastSeenScanId = useRef<number | null | undefined>(undefined);

  const updateFolderCount = useCallback((count: number) => {
    folderCountRevision.current += 1;
    setFolderCount(count);
  }, []);

  useEffect(() => {
    const refreshFolders = () => setFolderWatch((value) => value + 1);
    window.addEventListener("focus", refreshFolders);
    return () => window.removeEventListener("focus", refreshFolders);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let alive = true;

    const load = async () => {
      const revision = folderCountRevision.current;
      try {
        const folders = await listMediaFolders(controller.signal);
        if (alive && revision === folderCountRevision.current) {
          setFolderCount(folders.length);
        }
      } catch (failure) {
        if (!alive || isAborted(failure) || revision !== folderCountRevision.current) {
          return;
        }
        setFolderCount(null);
        timer = setTimeout(() => void load(), pollInterval);
      }
    };

    void load();
    return () => {
      alive = false;
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [folderWatch]);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let alive = true;

    const tick = async () => {
      let current: Scan | null;
      try {
        current = await getCurrentScan(controller.signal);
        if (!alive) return;
        const previousScanId = lastSeenScanId.current;
        lastSeenScanId.current = current?.id ?? null;
        setScan(current);
        setPollError(null);

        if (
          previousScanId !== undefined &&
          current !== null &&
          current.id !== previousScanId &&
          current.state !== "running"
        ) {
          setFinished(current);
        }
      } catch (failure) {
        if (alive && !isAborted(failure)) {
          setPollError(errorMessage(failure));
          if (
            observedRunningScanId.current !== null ||
            (recoveryBaselineScanId.current !== undefined &&
              recoveryPollsLeft.current > 0)
          ) {
            if (recoveryBaselineScanId.current !== undefined) {
              recoveryPollsLeft.current -= 1;
            }
            timer = setTimeout(() => void tick(), pollInterval);
          }
        }
        return;
      }

      const recoveryBaseline = recoveryBaselineScanId.current;
      const recovered =
        recoveryBaseline !== undefined &&
        current !== null &&
        (current.state === "running" || current.id !== recoveryBaseline);
      if (recovered) {
        recoveryBaselineScanId.current = undefined;
        recoveryPollsLeft.current = 0;
        setStartError(null);
      }

      if (current === null) {
        if (recoveryBaseline !== undefined && recoveryPollsLeft.current > 0) {
          recoveryPollsLeft.current -= 1;
          timer = setTimeout(() => void tick(), pollInterval);
        }
        return;
      }
      if (current.state === "running") {
        observedRunningScanId.current = current.id;
        timer = setTimeout(() => void tick(), pollInterval);
        return;
      }

      const completedObservedScan = observedRunningScanId.current === current.id;
      const completedRequestedScan = requestedScanId.current === current.id;
      if (completedObservedScan || completedRequestedScan || recovered) {
        setFinished(current);
      }
      if (completedObservedScan) observedRunningScanId.current = null;
      if (completedRequestedScan) requestedScanId.current = null;
      if (!recovered && recoveryBaseline !== undefined && recoveryPollsLeft.current > 0) {
        recoveryPollsLeft.current -= 1;
        timer = setTimeout(() => void tick(), pollInterval);
      }
    };

    void tick();
    return () => {
      alive = false;
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [watch]);

  const start = useCallback(() => {
    if (folderCount === null || folderCount === 0) return;
    const baselineScanId = scan?.id ?? null;
    setStarting(true);
    setStartError(null);
    recoveryBaselineScanId.current = undefined;
    recoveryPollsLeft.current = 0;
    void (async () => {
      try {
        const started = await startScan();
        requestedScanId.current = started.id;
        setScan(started);
        setPollError(null);
        if (started.state === "running") {
          observedRunningScanId.current = started.id;
        } else {
          requestedScanId.current = null;
          setFinished(started);
        }
        setWatch((value) => value + 1);
      } catch (failure) {
        if (
          failure instanceof RequestFailed &&
          failure.code === "media_folders_not_configured"
        ) {
          updateFolderCount(0);
        }
        setStartError(`取り込みを始められません: ${errorMessage(failure)}`);
        recoveryBaselineScanId.current = baselineScanId;
        recoveryPollsLeft.current = recoveryPollLimit;
        setWatch((value) => value + 1);
      } finally {
        setStarting(false);
      }
    })();
  }, [folderCount, scan?.id, updateFolderCount]);

  const refresh = useCallback(() => {
    setWatch((value) => value + 1);
    setFolderWatch((value) => value + 1);
  }, []);

  const error = startError ?? pollError;

  const value = useMemo<ScanContextValue>(
    () => ({
      scan,
      error,
      starting,
      running: starting || scan?.state === "running",
      canStart: folderCount !== null && folderCount > 0,
      start,
      refresh,
      setFolderCount: updateFolderCount,
      finished,
    }),
    [error, finished, folderCount, refresh, scan, start, starting, updateFolderCount],
  );

  return <ScanContext.Provider value={value}>{children}</ScanContext.Provider>;
}

/** describeScan は状態を一行にする（ツールチップ・読み上げ用）。 */
export function describeScan(value: ScanContextValue): string {
  const { scan, error } = value;
  if (error !== null) return error;
  if (value.starting) return "取り込みを開始しています…";
  if (scan === null) return "まだ取り込んでいません";
  const failed = scan.failed > 0 ? `（${String(scan.failed)} 件失敗）` : "";
  switch (scan.state) {
    case "running":
      return scan.total > 0
        ? `取り込み中 ${String(scan.completed)} / ${String(scan.total)}${failed}`
        : "取り込み中…";
    case "done":
      return scan.completed > 0
        ? `前回 ${String(scan.completed)} 件を取り込みました${failed}`
        : `前回の取り込みで変化はありませんでした${failed}`;
    case "failed":
      return `取り込みに失敗しました: ${scan.error ?? "理由は記録されていません"}`;
  }
}
