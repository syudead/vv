import type { Progress } from "./client";
import { applyProgressToListSnapshot } from "./listSnapshot";

type Listener = (videoId: number, progress: Progress) => void;

const listeners = new Set<Listener>();

/** sent は送った保存の通し番号で、送った順に増える。 */
let sent = 0;
/** applied は動画ごとに、反映済みの保存の通し番号である。 */
const applied = new Map<number, number>();

/**
 * nextProgressSequence は保存を送る直前に呼び、その保存の通し番号を返す。
 * 応答の届く順は送った順と限らないので、反映するときにこの番号で比べる。
 */
export function nextProgressSequence(): number {
  sent += 1;
  return sent;
}

/**
 * recordSavedProgress はサーバーが受け付けた再生位置を、画面が持っている一覧へ
 * 知らせる。
 *
 * 一覧は再生画面へ移る前の中身を控えから復元するので、そのままでは見終えた
 * 動画も未視聴のまま見える。完了の判定はサーバーが決めるので、応答の
 * Progress をそのまま使う。離脱時の送信は一覧が戻ってから応答することがあるため、
 * 控えを書き換えるだけでなく、表示中の一覧にも知らせる。
 */
export function recordSavedProgress(
  videoId: number,
  progress: Progress,
  sequence: number,
): void {
  // 後から送った保存の応答が先に届いていたら、遅れて届いた古い応答は捨てる。
  if (sequence <= (applied.get(videoId) ?? 0)) return;
  applied.set(videoId, sequence);
  applyProgressToListSnapshot(videoId, progress);
  for (const listener of listeners) listener(videoId, progress);
}

/** subscribeProgress は保存された再生位置を受け取る。戻り値で購読をやめる。 */
export function subscribeProgress(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
