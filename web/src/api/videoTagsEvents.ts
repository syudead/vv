import type { TagRef } from "./client";
import { applyTagToListSnapshot } from "./listSnapshot";

/**
 * videoTagsEvents は動画への付け外しの結果を、progressEvents と同じ形で画面へ
 * 知らせる（issue 267、Plan の Structural Decisions 7）。読み込み済みの一覧の項目と
 * listSnapshot の控えの tags を、一覧を取り直さずに直す。
 */
type Listener = (
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
) => void;

const listeners = new Set<Listener>();

/** sent は要求を送る直前に払い出す通し番号で、送った順に増える。 */
let sent = 0;

/**
 * applied は `動画id\0タグid` の組ごとに、反映済みの要求の通し番号を持つ。
 * 同じ動画の同じタグへの操作（例: 付けてすぐ外す）が2つ飛び、応答が送った順と
 * 違う順で届いても、後から送った操作の結果を先に届いた古い応答で巻き戻さない
 * ようにする（progressEvents.ts の sequence と同じ仕組み）。
 */
const applied = new Map<string, number>();

function key(videoId: number, tagId: number): string {
  return `${String(videoId)}\0${String(tagId)}`;
}

/**
 * nextVideoTagsSequence は要求を送る直前に呼び、その要求の通し番号を返す。
 * 応答の届く順は送った順と限らないので、反映するときにこの番号で比べる。
 */
export function nextVideoTagsSequence(): number {
  sent += 1;
  return sent;
}

/**
 * recordAppliedVideoTags はサーバーが受け付けた付け外しを、控えと画面へ知らせる。
 * `videoIds` は要求で送った全件（サーバーが数える `applied` はライブラリに
 * あるものだけだが、無い分は控えにも一覧にも現れないので絞り込まなくてよい）。
 *
 * `sequence` は `nextVideoTagsSequence` が返した、この要求の通し番号
 * （呼び出し元が送信の直前に払い出す）。動画・タグの組ごとに、それより新しい
 * 操作が既に反映されていれば、その組はここでの反映から外す（N1）。
 */
export function recordAppliedVideoTags(
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
  sequence: number,
): void {
  const fresh = videoIds.filter((videoId) => {
    const k = key(videoId, tag.id);
    if (sequence <= (applied.get(k) ?? 0)) return false;
    applied.set(k, sequence);
    return true;
  });
  if (fresh.length === 0) return;
  applyTagToListSnapshot(fresh, tag, action);
  for (const listener of listeners) listener(fresh, tag, action);
}

/** subscribeVideoTags は付け外しの結果を受け取る。戻り値で購読をやめる。 */
export function subscribeVideoTags(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
