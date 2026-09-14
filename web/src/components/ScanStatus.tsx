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
 * watching は「終わりを見届ける取り込みがある」ことを表す。
 *
 * `GET /api/scans/current` は実行中のものが無ければ**最後に終わったもの**を
 * 返す。最初の巡回で done が返るのはふつうの状態であって「いま終わった」の
 * ではないので、これを知らせると一覧を開くたびに読み直しが走り、一覧の復元
 * （FR-016）が毎回捨てられる。実行中を見たとき、または利用者が取り込みを
 * 促したときにだけ、終わりを知らせる対象にする。
 *
 * **この印は部品より長く生きる。** 一覧で実行中を見たあと再生画面へ移ると
 * ScanStatus は捨てられるので、部品の中に持つと「見ていない間に終わった
 * 取り込み」を知らせそこなう ── 戻ってきた一覧が控えのまま古い件数を出し
 * 続ける。タブを読み込み直せば消える寿命でよく、それは控え
 * （api/listSnapshot.ts）と同じ寿命である。
 */
let watching = false;

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
  // 「始められなかった」は状態の取得に失敗したのとは別の失敗である。
  // 同じ入れ物に入れると、押した直後の巡回が成功しただけで消えてしまい、
  // 押した利用者には始まったように見える。
  const [startError, setStartError] = useState<string | null>(null);
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
        watching = true;
        timer = setTimeout(() => void tick(), pollInterval);
        return;
      }
      if (!watching) {
        // 前回の取り込みが done のまま残っているだけである。
        return;
      }
      // 走り終わった直後は、一覧に新しい動画が並んでいる。見ていない間に
      // 終わっていた場合もここへ来る（印が部品より長く生きるため）。
      watching = false;
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
    setStartError(null);
    void (async () => {
      try {
        // 利用者が促した取り込みは、最初の巡回で既に終わっていても
        // 終わりを知らせる（小さなライブラリでは巡回より先に終わる）。
        //
        // 印を立てるのは**始まったあと**である。始める前に立てると、要求が
        // 失敗したときに印だけが残り、次にこの部品が作られたときへ持ち越して
        // しまう ── 前回の done を「いま終わった」と誤り、控えを捨てる。
        // 見張り直すのは下の finally からなので、ここで立てれば間に合う。
        const started = await startScan();
        watching = true;
        setScan(started);
        setError(null);
      } catch (failure) {
        setStartError(errorMessage(failure));
      } finally {
        setStarting(false);
        // 成否によらず見張り直す。**要求の失敗は「始まらなかった」ことを
        // 保証しない** ── サーバーが取り込みを始めたあとで応答だけが失われる
        // ことがある。巡回して実行中を見つければ、上の tick がそこから印を
        // 立てて終わりまで見届ける。始まっていなければ前回の done が返る
        // だけで、印は立たない。
        setWatch((value) => value + 1);
      }
    })();
  }, []);

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted">
      {/* 状態は 1 行の文言で示す。進行中は describe が「済んだ数 / 総数」を
          返す（contracts/screen-states.md 1.「固定の帯」）。 */}
      <span className={error !== null || startError !== null ? "text-danger" : undefined}>
        {describe(scan, error, startError)}
      </span>

      <button
        type="button"
        onClick={onStart}
        disabled={starting || scan?.state === "running"}
        // 押せる要素は --size-tap（44px）四方以上にする（FR-022）。
        className="min-h-[var(--size-tap)] min-w-[var(--size-tap)] rounded-control border border-border px-3 text-sm text-body disabled:cursor-not-allowed disabled:opacity-50"
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
function describe(
  scan: Scan | null,
  error: string | null,
  startError: string | null,
): string {
  // 押した操作が失敗したことを先に伝える。利用者にとっては、いま押した
  // ものがどうなったかのほうが、巡回の成否より知りたいことである。
  if (startError !== null) {
    return `取り込みを始められません: ${startError}`;
  }
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
