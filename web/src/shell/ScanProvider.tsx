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
  getProcessing,
  isAborted,
  listMediaFolders,
  type Processing,
  RequestFailed,
  startScan,
  type Scan,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";

export interface ScanContextValue {
  scan: Scan | null;
  /** current scan の初回取得が完了している。 */
  loaded: boolean;
  /**
   * 取り込みの段階（解析・サムネイル・プレビュー）ごとに残っている仕事の数。
   * まだ取得していなければ null。
   */
  processing: Processing | null;
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

/** processingRemaining は全段階の残りの合計である。未取得なら 0。 */
export function processingRemaining(processing: Processing | null): number {
  if (processing === null) return 0;
  return processing.probe + processing.thumbnail + processing.preview;
}

/**
 * ScanProvider は取り込み状態をひとつ持つ。
 *
 * 状態は最初に1度取得し、その後はサーバーからの変化の知らせ（`/api/events`）で
 * 更新する。一定間隔では問い合わせない。つなぎ直したとき、ウィンドウへ戻った
 * とき、利用者が更新を求めたときは取り直す。トップバーのボタンとライブラリの
 * 再読込が同じ状態を見る。
 */
export function ScanProvider({ children }: { children: ReactNode }) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [processing, setProcessing] = useState<Processing | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [startError, setStartError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [finished, setFinished] = useState<Scan | null>(null);
  const [folderCount, setFolderCount] = useState<number | null>(null);
  const folderCountRevision = useRef(0);
  const requestedScanId = useRef<number | null>(null);
  const observedRunningScanId = useRef<number | null>(null);
  const recoveryBaselineScanId = useRef<number | null | undefined>(undefined);
  const lastSeenScanId = useRef<number | null | undefined>(undefined);
  // 取得と変化の知らせは並行する。知らせの方が新しいことがあるので、取得を
  // 始めたあとに知らせを受けたら、その取得の応答は捨てる。
  const scanRevision = useRef(0);
  const processingRevision = useRef(0);

  const updateFolderCount = useCallback((count: number) => {
    folderCountRevision.current += 1;
    setFolderCount(count);
  }, []);

  /**
   * apply はサーバーから得たスキャンを取り込み、「終わったのを見た」を決める。
   * 取得の応答と変化の知らせのどちらから来ても同じ規則で扱う。
   */
  const apply = useCallback((current: Scan | null) => {
    const previousScanId = lastSeenScanId.current;
    lastSeenScanId.current = current?.id ?? null;
    setScan(current);
    setLoaded(true);
    setLoadError(null);
    if (current === null) return;

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

  const loadFolders = useCallback(async (signal: AbortSignal) => {
    const revision = folderCountRevision.current;
    try {
      const folders = await listMediaFolders(signal);
      if (revision === folderCountRevision.current) setFolderCount(folders.length);
    } catch (failure) {
      if (isAborted(failure) || revision !== folderCountRevision.current) return;
      setFolderCount(null);
    }
  }, []);

  /**
   * loadScan はスキャンと段階ごとの残りを取り直す。遅れて届いた古い応答は捨てる。
   */
  const inFlight = useRef<AbortController | null>(null);
  const loadScan = useCallback(() => {
    inFlight.current?.abort();
    const controller = new AbortController();
    inFlight.current = controller;
    scanRevision.current += 1;
    processingRevision.current += 1;
    const scanAt = scanRevision.current;
    const processingAt = processingRevision.current;

    void (async () => {
      try {
        const nextScan = await getCurrentScan(controller.signal);
        if (scanAt === scanRevision.current) apply(nextScan);
      } catch (failure) {
        if (scanAt !== scanRevision.current || isAborted(failure)) return;
        // 最後に得た状態は捨てない。つなぎ直しやウィンドウへの復帰で取り直す。
        setLoadError(errorMessage(failure));
      }
    })();
    void (async () => {
      try {
        const nextProcessing = await getProcessing(controller.signal);
        if (processingAt === processingRevision.current) setProcessing(nextProcessing);
      } catch {
        // 残りの数は補助の情報なので、取れなくても取り込みの状態は示せる。
        // 最後に得た数を残し、次の知らせか取り直しを待つ。
      }
    })();
  }, [apply]);

  /** load はフォルダの件数も含めて、今の状態をまとめて取り直す。 */
  const foldersInFlight = useRef<AbortController | null>(null);
  const load = useCallback(() => {
    foldersInFlight.current?.abort();
    const controller = new AbortController();
    foldersInFlight.current = controller;
    void loadFolders(controller.signal);
    loadScan();
  }, [loadFolders, loadScan]);

  useEffect(() => {
    // 購読してから取得する。取得のあとに起きた変化を取りこぼさない。
    const unsubscribe = subscribeServerEvents({
      scan: (next) => {
        scanRevision.current += 1;
        apply(next);
      },
      processing: (next) => {
        processingRevision.current += 1;
        setProcessing(next);
      },
      open: load,
    });
    load();
    window.addEventListener("focus", load);
    return () => {
      unsubscribe();
      window.removeEventListener("focus", load);
      inFlight.current?.abort();
      foldersInFlight.current?.abort();
    };
  }, [apply, load]);

  const start = useCallback(() => {
    if (folderCount === null || folderCount === 0) return;
    const baselineScanId = scan?.id ?? null;
    setStarting(true);
    setStartError(null);
    recoveryBaselineScanId.current = undefined;
    void (async () => {
      try {
        const started = await startScan();
        requestedScanId.current = started.id;
        if (started.state === "running") {
          observedRunningScanId.current = started.id;
        }
        apply(started);
      } catch (failure) {
        if (
          failure instanceof RequestFailed &&
          failure.code === "media_folders_not_configured"
        ) {
          updateFolderCount(0);
        }
        setStartError(`取り込みを始められません: ${errorMessage(failure)}`);
        // 応答だけを失い、取り込み自体は始まっていることがある。変化の知らせか
        // 次の取得でそれを見たら、失敗の表示を消して追跡する。
        recoveryBaselineScanId.current = baselineScanId;
      } finally {
        setStarting(false);
      }
      // 開始の直後に終わる小さな取り込みもある。知らせを待たずに1度取り直す。
      loadScan();
    })();
  }, [apply, folderCount, loadScan, scan?.id, updateFolderCount]);

  const error = startError ?? loadError;

  const value = useMemo<ScanContextValue>(
    () => ({
      scan,
      loaded,
      processing,
      error,
      starting,
      running: starting || scan?.state === "running",
      canStart: folderCount !== null && folderCount > 0,
      start,
      refresh: load,
      setFolderCount: updateFolderCount,
      finished,
    }),
    [
      error,
      finished,
      folderCount,
      load,
      loaded,
      processing,
      scan,
      start,
      starting,
      updateFolderCount,
    ],
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
    case "done": {
      const remaining = processingRemaining(value.processing);
      if (remaining > 0) return `取り込んだ動画を準備中（残り ${String(remaining)} 件）`;
      return scan.completed > 0
        ? `前回 ${String(scan.completed)} 件を取り込みました${failed}`
        : `前回の取り込みで変化はありませんでした${failed}`;
    }
    case "failed":
      return `取り込みに失敗しました: ${scan.error ?? "理由は記録されていません"}`;
  }
}
