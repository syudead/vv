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
  startScan,
  type Scan,
  type ScanActivity,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { useAudience } from "../auth/audience";
import { errorText, t, type UiText } from "../i18n";

export interface ScanContextValue {
  scan: Scan | null;
  /** current scan の初回取得が完了している。 */
  loaded: boolean;
  /**
   * 今の処理。取り込み中に `Scan.activity` が一瞬無くなっても、同じ取り込みのあいだは
   * 直前の値を残す（specs/024-import-progress/ui-design.md「Words」）。
   */
  activity: ScanActivity | null;
  /** 状態取得や開始の失敗。 */
  error: UiText | null;
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

/**
 * ScanControlsValue は、取り込みの進み（済みの本数・今の処理）を読まない画面が使う部分である。
 * 値は取り込みの始まりと終わり、フォルダの件数の変化でだけ変わる。取り込み中は1ファイルごとに
 * `scan` の知らせが届くので、`useScan` を読む一覧の画面は、そのたびに丸ごと描き直される
 * （issue 674）。一覧の画面はこちらを読む。
 */
export type ScanControlsValue = Pick<
  ScanContextValue,
  "running" | "canStart" | "start" | "refresh" | "setFolderCount" | "finished"
>;

/**
 * dedupeLoadMs は、取り直しの求めを途中の取得にまとめる時間である。これより前に始めた
 * 取得がまだ返らなければ、打ち切って取り直す。
 */
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

/** useScanControls は取り込みの開始・取り直し・「終わったのを見た」だけを読む。 */
export function useScanControls(): ScanControlsValue {
  const value = useContext(ScanControlsContext);
  if (value === null) {
    throw new Error("useScanControls must be used inside ScanProvider");
  }
  return value;
}

/** inProgress は、取り込みがまだ終わっていない（走査中か、準備が残る）かを返す。 */
export function inProgress(scan: Scan): boolean {
  return scan.status === "finding" || scan.status === "running";
}

/**
 * nextActivity は、新しい `Scan` を受けたときに示す今の処理を決める。同じ取り込みが
 * 終わっていないあいだは、activity が無くても直前の値を残す。
 */
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
 * ScanProvider は取り込み状態をひとつ持つ。
 *
 * 状態は最初に1度取得し、その後はサーバーからの変化の知らせ（`/api/events`）で
 * 更新する。一定間隔では問い合わせない。つなぎ直したとき、ウィンドウへ戻った
 * とき、利用者が更新を求めたときは取り直す。トップバーのボタンとライブラリの
 * 再読込が同じ状態を見る。
 *
 * 取り込みは所有者だけのものなので、ゲストとして描くときは状態を取りに行かず
 * （`GET /api/scans/current` を呼ばない）、`/api/events` も購読しない。取り込みの
 * 状態は「まだ取り込んでいない」のまま動かず、開始も取り直しも何もしない
 * （specs/016-single-account-auth/ui-design.md「Top bar」）。
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
  const [folderCount, setFolderCount] = useState<number | null>(null);
  const folderCountRevision = useRef(0);
  const requestedScanId = useRef<number | null>(null);
  const observedRunningScanId = useRef<number | null>(null);
  const recoveryBaselineScanId = useRef<number | null | undefined>(undefined);
  const lastSeenScanId = useRef<number | null | undefined>(undefined);
  // 取得と変化の知らせは並行する。知らせの方が新しいことがあるので、取得を
  // 始めたあとに知らせを受けたら、その取得の応答は捨てる。
  const scanRevision = useRef(0);

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
    setActivity((previous) => nextActivity(previous, current));
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

  // 直近の取得が失敗したか。最初の接続で取り直すかを決める。
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

  /** loadScan は直近の取り込みを取り直す。遅れて届いた古い応答は捨てる。 */
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
        // 最後に得た状態は捨てない。つなぎ直しやウィンドウへの復帰で取り直す。
        loadFailed.current = true;
        setLoadError(errorText(failure));
      } finally {
        if (inFlight.current === controller) inFlight.current = null;
      }
    })();
  }, [apply]);

  /** load はフォルダの件数も含めて、今の状態をまとめて取り直す。 */
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
   * loadUnlessLoading は、直前（`dedupeLoadMs` 以内）に始めた取得の途中でなければ取り直す。
   * 途中なら、その応答が今の状態を運ぶ。画面を開いたときの取り直しと、この部品の最初の
   * 取得が重なって、同じ取得を2回続けて送らないようにする（子の画面の effect は親より先に
   * 走る。issue 674）。それより前に始めた取得がまだ返らないなら、返らないまま止まっている
   * かもしれないので、`load` で打ち切って取り直す。
   */
  const loadUnlessLoading = useCallback(() => {
    // 打ち切った取得は応答を捨てるので、途中の取得に数えない（effect の後始末のすぐあとに
    // もう一度走る StrictMode や、owner の切り替えで取り直しを落とさない）。
    const pending = (controller: AbortController | null) =>
      controller !== null && !controller.signal.aborted;
    const loading = pending(inFlight.current) || pending(foldersInFlight.current);
    if (loading && Date.now() - loadStartedAt.current < dedupeLoadMs) return;
    load();
  }, [load]);

  useEffect(() => {
    if (!owner) return;
    // 購読してから取得する。取得のあとに起きた変化を取りこぼさない。
    const unsubscribe = subscribeServerEvents({
      scan: (next) => {
        scanRevision.current += 1;
        apply(next);
      },
      // つないだ直後にサーバーは今の状態を `scan` で送るので、最初の接続では、直前の取得が
      // 失敗していなければ取り直さない。つなぎ直しでは、切れていた間のフォルダの件数の
      // 変化も取り直す。
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

  const start = useCallback(() => {
    if (!owner || folderCount === null || folderCount === 0) return;
    const baselineScanId = scan?.id ?? null;
    setStarting(true);
    setStartError(null);
    recoveryBaselineScanId.current = undefined;
    // 開始の応答より先に、変化の知らせが届くことがある（すぐ終わる小さな
    // 取り込み）。そのときは知らせの方が新しいので、応答の状態で戻さない。
    const requestedAt = scanRevision.current;
    void (async () => {
      try {
        const started = await startScan();
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
        setStartError(t.shell.scan.startFailed(errorText(failure)));
        // 応答だけを失い、取り込み自体は始まっていることがある。変化の知らせか
        // 次の取得でそれを見たら、失敗の表示を消して追跡する。
        recoveryBaselineScanId.current = baselineScanId;
      } finally {
        setStarting(false);
      }
      // 開始の直後に終わる小さな取り込みもある。知らせを待たずに1度取り直す。
      loadScan();
    })();
  }, [apply, folderCount, loadScan, owner, scan?.id, updateFolderCount]);

  const error = startError ?? loadError;

  const value = useMemo<ScanContextValue>(
    () => ({
      scan,
      loaded,
      activity: activity?.activity ?? null,
      error,
      starting,
      running: starting || scan?.state === "running",
      canStart: folderCount !== null && folderCount > 0,
      start,
      refresh,
      setFolderCount: updateFolderCount,
      finished,
    }),
    [
      activity,
      error,
      finished,
      folderCount,
      loaded,
      refresh,
      scan,
      start,
      starting,
      updateFolderCount,
    ],
  );

  const running = starting || scan?.state === "running";
  const canStart = folderCount !== null && folderCount > 0;
  const controls = useMemo<ScanControlsValue>(
    () => ({
      running,
      canStart,
      start,
      refresh,
      setFolderCount: updateFolderCount,
      finished,
    }),
    [canStart, finished, refresh, running, start, updateFolderCount],
  );

  return (
    <ScanControlsContext.Provider value={controls}>
      <ScanContext.Provider value={value}>{children}</ScanContext.Provider>
    </ScanControlsContext.Provider>
  );
}
