import { useCallback, useEffect, useState } from "react";

import {
  errorMessage,
  getCurrentScan,
  isAborted,
  startScan,
  type Scan,
} from "../api/client";

/** pollInterval は取り込み中に状態を見に行く間隔である。 */
const pollInterval = 2000;

/**
 * ScanStatus は取り込みの状態を出し、再取り込みを促せるようにする
 * （FR-006）。
 *
 * 進行中は「どれだけ残っているか」が分かる形にする。件数だけでは終わりが
 * 見えないため、総数と済んだ数を並べる。
 */
export default function ScanStatus({ onFinished }: { onFinished?: () => void }) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  // 取り込みを促したら見張り直す。押した直後に状態が動くので、次の巡回を
  // 待たずに追いかける。
  const [watch, setWatch] = useState(0);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    try {
      const current = await getCurrentScan(signal);
      setScan(current);
      setError(null);
      return current;
    } catch (failure) {
      if (!isAborted(failure)) {
        setError(errorMessage(failure));
      }
      return null;
    }
  }, []);

  // 取り込み中だけ見に行く。終わっていれば止めるので、待機中のアプリケーションが
  // 要求を出し続けることはない。
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let running = true;

    const tick = async () => {
      const current = await refresh(controller.signal);
      if (!running) {
        return;
      }
      if (current === null) {
        return;
      }
      if (current.state === "running") {
        timer = setTimeout(() => void tick(), pollInterval);
        return;
      }
      // 走り終わった直後は、一覧に新しい動画が並んでいる。
      onFinished?.();
    };

    void tick();

    return () => {
      running = false;
      controller.abort();
      if (timer !== undefined) {
        clearTimeout(timer);
      }
    };
  }, [onFinished, refresh, watch]);

  const onStart = useCallback(() => {
    setStarting(true);
    void (async () => {
      try {
        setScan(await startScan());
        setError(null);
        setWatch((value) => value + 1);
      } catch (failure) {
        setError(errorMessage(failure));
      } finally {
        setStarting(false);
      }
    })();
  }, []);

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-600">
      <span>{describe(scan, error)}</span>

      <button
        type="button"
        onClick={onStart}
        disabled={starting || scan?.state === "running"}
        className="rounded border border-neutral-300 px-2.5 py-1 text-sm text-neutral-800 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-50"
      >
        取り込む
      </button>
    </div>
  );
}

/**
 * describe は取り込みの状態を1行で言い表す。
 *
 * completed は「この取り込みで反映した数」であって、ライブラリの総数では
 * ない（総数は一覧側が出す）。変化が無ければ 0 になるので、「N 件」とだけ
 * 書くと蔵書が 0 本になったように読める。何の数かが分かる文言にする。
 */
function describe(scan: Scan | null, error: string | null): string {
  if (error !== null) {
    return `取り込みの状態を取得できません: ${error}`;
  }
  if (scan === null) {
    return "まだ取り込んでいません";
  }

  const failed =
    scan.failed > 0 ? `・${String(scan.failed)} 件は取り込めませんでした` : "";

  switch (scan.state) {
    case "running":
      return scan.total > 0
        ? `取り込み中 ${String(scan.completed)} / ${String(scan.total)} 件${failed}`
        : "取り込み中…";
    case "done":
      return scan.completed > 0
        ? `前回の取り込みで ${String(scan.completed)} 件を反映${failed}`
        : `前回の取り込みで変化はありませんでした${failed}`;
    case "failed":
      return `取り込みに失敗しました: ${scan.error ?? "理由は記録されていません"}`;
  }
}
