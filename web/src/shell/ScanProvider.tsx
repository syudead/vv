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
  startScan,
  type Scan,
} from "../api/client";

const pollInterval = 2000;

export interface ScanContextValue {
  scan: Scan | null;
  /** 状態取得や開始の失敗。 */
  error: string | null;
  starting: boolean;
  running: boolean;
  start: () => void;
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
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [finished, setFinished] = useState<Scan | null>(null);
  const [watch, setWatch] = useState(0);
  const firstSight = useRef(true);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let alive = true;

    const tick = async () => {
      let current: Scan | null;
      try {
        current = await getCurrentScan(controller.signal);
        setScan(current);
        setError(null);
      } catch (failure) {
        if (!isAborted(failure)) setError(errorMessage(failure));
        return;
      }
      if (!alive) return;

      const initial = firstSight.current;
      firstSight.current = false;

      if (current === null) return;
      if (current.state === "running") {
        timer = setTimeout(() => void tick(), pollInterval);
        return;
      }
      if (!initial) {
        setFinished(current);
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
    setStarting(true);
    void (async () => {
      try {
        setScan(await startScan());
        setError(null);
      } catch (failure) {
        setError(`取り込みを始められません: ${errorMessage(failure)}`);
      } finally {
        setStarting(false);
        setWatch((value) => value + 1);
      }
    })();
  }, []);

  const value = useMemo<ScanContextValue>(
    () => ({
      scan,
      error,
      starting,
      running: starting || scan?.state === "running",
      start,
      finished,
    }),
    [error, finished, scan, start, starting],
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
