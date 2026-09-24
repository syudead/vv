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

/**
 * recordAppliedVideoTags はサーバーが受け付けた付け外しを、控えと画面へ知らせる。
 * `videoIds` は要求で送った全件（サーバーが数える `applied` はライブラリに
 * あるものだけだが、無い分は控えにも一覧にも現れないので絞り込まなくてよい）。
 */
export function recordAppliedVideoTags(
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
): void {
  applyTagToListSnapshot(videoIds, tag, action);
  for (const listener of listeners) listener(videoIds, tag, action);
}

/** subscribeVideoTags は付け外しの結果を受け取る。戻り値で購読をやめる。 */
export function subscribeVideoTags(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
