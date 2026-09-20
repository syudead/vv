import { useCallback, useEffect, useRef, useState } from "react";

import {
  errorMessage,
  getCurrentScan,
  isAborted,
  startScan,
  type Scan,
} from "../api/client";
import Icon from "../layout/icons";

/** pollInterval は取り込み中に状態を見に行く間隔である。 */
const pollInterval = 2000;

/**
 * SeenScan は「押す前に何が見えていたか」である。3 通りを区別する。
 *
 * | 値 | 意味 |
 * | --- | --- |
 * | `undefined` | まだ状態を取れていない（何があるか分からない） |
 * | `null` | 取れた。取り込みは 1 つも無い |
 * | 数値 | 取れた。その id の取り込みが最後である |
 *
 * `undefined` と `null` を混ぜると、最初の状態が届く前に「取り込む」を押して
 * 失敗したとき、**もとからあった取り込みが見えただけ**で「新しい取り込みが
 * 始まった」と誤り、失敗の文言を引っ込めてしまう。利用者には始まったように
 * 見えるが、実際には何も始まっていない。
 */
type SeenScan = number | null | undefined;

/**
 * ScanStatus は取り込みの状態を出し、再取り込みを促せるようにする
 * （FR-006）。
 *
 * 進行中は「どれだけ残っているか」が分かる形にする。件数だけでは終わりが
 * 見えないため、総数と済んだ数を並べる。
 *
 * **終わりの判断はここでしない。** `GET /api/scans/current` は実行中のものが
 * 無ければ最後に終わったものを返すので、この部品から見れば「終わっている
 * 取り込みがある」としか言えない。それが呼び出し側にとって新しいものかどうか
 * （＝一覧を読み直すべきか）は、呼び出し側が自分の知っている取り込みと
 * 見比べて決める。ここで決めようとすると、画面をまたいで生きる印を持つ
 * ことになり、印を立てそこねる／降ろしそこねる経路が増える。
 */
export default function ScanStatus({
  onFinished,
}: {
  /**
   * 終わっている取り込みを観測するたびに呼ぶ。同じものを何度も渡しうる。
   *
   * `firstSight` は、この部品が**最初に見た状態**が既に終わっていたことを
   * 表す。呼び出し側が一覧を読んだのとほぼ同時なので、その取り込みの結果は
   * すでに一覧へ入っている。2 回目以降の観測は「読んだあとに終わった」もので、
   * 一覧はそれを映していない。
   */
  onFinished?: (scan: Scan, firstSight: boolean) => void;
}) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [error, setError] = useState<string | null>(null);
  /**
   * startFailure は「押したのに始められなかった」ことである。
   *
   * 状態の取得の失敗とは別に持つ。同じ入れ物に入れると、押した直後の巡回が
   * 成功しただけで消えてしまい、押した利用者には始まったように見える。
   *
   * `sinceScanId` は失敗した時点で見えていた取り込みの id である。**要求の
   * 失敗は「始まらなかった」ことを保証しない**ので、そのあとに実行中が見えたり
   * 別の取り込みが見えたりしたら、この言い分のほうが誤りだったことになる。
   * 消す判断にこの id が要る。
   */
  const [startFailure, setStartFailure] = useState<{
    message: string;
    since: SeenScan;
  } | null>(null);
  const [starting, setStarting] = useState(false);
  // 取り込みを促したら見張り直す。押した直後に状態が動くので、次の巡回を
  // 待たずに追いかける。
  const [watch, setWatch] = useState(0);

  // 直近に見えた取り込み。押した時点の「押す前の状態」を知るために持つ。
  // **「まだ見ていない」と「見たが 1 つも無い」を混ぜない**（SeenScan 参照）。
  const lastSeen = useRef<SeenScan>(undefined);

  // まだ一度も状態を見ていない。**この部品が作られてから 1 度だけ真**であり、
  // 「取り込む」で見張り直しても戻らない（押したあとの観測は、呼び出し側が
  // 一覧を読んだあとに起きたものだからである）。
  const atFirstSight = useRef(true);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    try {
      const current = await getCurrentScan(signal);
      setScan(current);
      setError(null);
      lastSeen.current = current === null ? null : current.id;

      if (current !== null) {
        setStartFailure((failure) => {
          if (failure === null) {
            return null;
          }
          // 走っているものがある = 「始められません」は誤りだった。
          if (current.state === "running") {
            return null;
          }
          // 押した時点で何が見えていたか分からない。もとからあった取り込みか
          // 新しく始まったものかを区別できないので、言い分はそのまま残す。
          if (failure.since === undefined) {
            return failure;
          }
          // 別の取り込みになった = やはり始まっていた。
          return current.id !== failure.since ? null : failure;
        });
      }
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
      // 一度でも状態を見たら、以後は「最初に見た状態」ではない。取れなかった
      // 回も数える（取れなかったことを根拠に「反映済み」とみなせない）。
      const firstSight = atFirstSight.current;
      atFirstSight.current = false;

      if (current === null) {
        return;
      }
      if (current.state === "running") {
        timer = setTimeout(() => void tick(), pollInterval);
        return;
      }
      // 終わっている取り込みを観測した。これが「いま終わった」ものなのか
      // 「前回の残り」なのかは、呼び出し側が id で見分ける。
      onFinished?.(current, firstSight);
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
    setStartFailure(null);
    void (async () => {
      // 押す前に見えていた取り込み。失敗の言い分をあとで見直すのに使う。
      const before = lastSeen.current;
      try {
        setScan(await startScan());
        setError(null);
      } catch (failure) {
        setStartFailure({ message: errorMessage(failure), since: before });
      } finally {
        setStarting(false);
        // 成否によらず見張り直す。**要求の失敗は「始まらなかった」ことを
        // 保証しない** ── サーバーが取り込みを始めたあとで応答だけが失われる
        // ことがある。押した直後に巡回し直せば、始まっていれば実行中が、
        // 既に終わっていれば新しい id が見えるので、どちらでも呼び出し側が
        // 気付ける。始まっていなければ前回の取り込みが返るだけである。
        setWatch((value) => value + 1);
      }
    })();
  }, []);

  return (
    // min-w-0 と break-words は、失敗の文言（サーバーからの理由がそのまま
    // 入りうる）で帯が横に伸びないようにする。狭い画面で横スクロールを
    // 生むのは、たいてい折り返せない長い 1 語である（FR-022 / SC-004）。
    <div className="flex min-w-0 flex-wrap items-center justify-end gap-x-3 gap-y-1 text-sm text-muted">
      {/* 状態は 1 行の文言で示す。進行中は describe が「済んだ数 / 総数」を
          返す（contracts/screen-states.md 1.「固定の帯」）。 */}
      <span
        className={
          (error !== null || startFailure !== null || scan?.state === "running"
            ? "min-w-0 break-words "
            : "sr-only ") + (error !== null || startFailure !== null ? "text-danger" : "")
        }
      >
        {describe(scan, error, startFailure?.message ?? null)}
      </span>

      <button
        type="button"
        onClick={onStart}
        disabled={starting || scan?.state === "running"}
        // 押せる要素は --size-tap（44px）四方以上にする（FR-022）。
        className="min-h-[var(--size-tap)] min-w-[var(--size-tap)] rounded-control border border-accent bg-accent px-3 text-sm font-semibold text-accent-ink outline-none transition-colors hover:bg-accent-surface hover:text-accent active:bg-surface-sunken active:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus disabled:cursor-not-allowed disabled:border-border disabled:bg-surface-raised disabled:text-muted motion-reduce:transition-none"
      >
        <Icon
          name="refresh"
          className={
            "mr-1.5 inline-block h-4 w-4 " +
            (starting || scan?.state === "running"
              ? "animate-spin motion-reduce:animate-none"
              : "")
          }
        />
        {starting ? "開始中" : scan?.state === "running" ? "更新中" : "更新"}
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
