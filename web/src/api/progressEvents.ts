import type { Progress } from "./client";
import { applyProgressToListSnapshot } from "./listSnapshot";

type Listener = (videoId: number, progress: Progress) => void;

const listeners = new Set<Listener>();

/**
 * recordSavedProgress はサーバーが受け付けた再生位置を、画面が持っている一覧へ
 * 知らせる。
 *
 * 一覧は再生画面へ移る前の中身を控えから復元するので、そのままでは見終えた
 * 動画も未視聴のまま見える。完了の判定はサーバーが決めるので、応答の
 * Progress をそのまま使う。離脱時の送信は一覧が戻ってから応答することがあるため、
 * 控えを書き換えるだけでなく、表示中の一覧にも知らせる。
 */
export function recordSavedProgress(videoId: number, progress: Progress): void {
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
