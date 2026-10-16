import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "./client";
import type { ItemPage } from "./readThrough";

/** PagedDetailState は 1 つのものを開いたページの読み込みの状態である。 */
export type PagedDetailState<D> =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "gone" }
  | { kind: "ready"; data: D };

/** MorePageState は続きのページの読み込みの状態である。 */
export type MorePageState = "idle" | "loading" | "failed";

/** isGone は応答が開いたものの無いこと（404）を表すかを返す。 */
function isGone(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/** PagedSource はページの読み方である。呼ぶたびに最新のものを使う。 */
export interface PagedSource<D extends ItemPage<I>, I> {
  /** 開いたものを読み、項目が `atLeast` 件そろうまで続きを辿る（readThrough）。 */
  read: (id: number, atLeast: number, signal: AbortSignal) => Promise<D>;
  /** 続きのページを 1 つ読む。 */
  more: (id: number, cursor: string, signal: AbortSignal) => Promise<ItemPage<I>>;
  /** 続きのページを今の項目の後ろへ足す（同じ項目は足さない）。 */
  append: (current: readonly I[], page: readonly I[]) => I[];
  /**
   * 読み込みを始める前に呼ぶ。返した関数は読めた中身を見せるかを答え、false ならその結果を
   * 捨てる（呼び手が読み直す）。省くといつも見せる。
   */
  beforeRead?: (quiet: boolean) => (data: D) => boolean;
  /** 続きのページを足したときに呼ぶ。 */
  onMore?: (items: readonly I[]) => void;
}

/**
 * usePagedDetail は 1 つのもの（グループ、スマートフォルダ）を開いたページの読み込みである。
 * `read(false)` は読み込み中を見せて読み、`read(true)` は静かに読み直す（並んでいる分を
 * 減らさず、失敗しても中身を残す）。続きは `loadMore` で 1 ページずつ足す。新しい読み込みを
 * 始めると、古い読み込みと続きのページの応答は捨てる。どの読み込みの 404 も無くなった
 * ことにする。`id` が変わったら（画面を作り直したら）読み込みを捨てる。
 */
export function usePagedDetail<D extends ItemPage<I>, I>(
  id: number | null,
  source: PagedSource<D, I>,
) {
  const [state, setState] = useState<PagedDetailState<D>>(
    id === null ? { kind: "gone" } : { kind: "loading" },
  );
  const [more, setMore] = useState<MorePageState>("idle");
  const stateRef = useRef(state);
  const reading = useRef<AbortController | null>(null);
  const latest = useRef(source);
  useEffect(() => {
    latest.current = source;
  });

  const show = useCallback((next: PagedDetailState<D>) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const read = useCallback(
    (quiet: boolean) => {
      if (id === null) return;
      reading.current?.abort();
      const controller = new AbortController();
      reading.current = controller;
      const current = stateRef.current;
      const atLeast = current.kind === "ready" ? current.data.items.length : 0;
      if (!quiet) show({ kind: "loading" });
      setMore("idle");
      const accept = latest.current.beforeRead?.(quiet);
      latest.current.read(id, atLeast, controller.signal).then(
        (data) => {
          if (controller.signal.aborted) return;
          reading.current = null;
          if (accept === undefined || accept(data)) show({ kind: "ready", data });
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          reading.current = null;
          if (isGone(error)) show({ kind: "gone" });
          else if (!quiet || stateRef.current.kind !== "ready") show({ kind: "failed" });
        },
      );
    },
    [id, show],
  );

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (id === null || current.kind !== "ready" || reading.current !== null) return;
    const cursor = current.data.nextCursor;
    if (cursor === undefined) return;
    const controller = new AbortController();
    reading.current = controller;
    setMore("loading");
    latest.current.more(id, cursor, controller.signal).then(
      (page) => {
        if (controller.signal.aborted) return;
        reading.current = null;
        const now = stateRef.current;
        if (now.kind !== "ready") return;
        setMore("idle");
        latest.current.onMore?.(page.items);
        show({
          kind: "ready",
          data: {
            ...now.data,
            items: latest.current.append(now.data.items, page.items),
            nextCursor: page.nextCursor,
          },
        });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        reading.current = null;
        setMore(isGone(error) ? "idle" : "failed");
        if (isGone(error)) show({ kind: "gone" });
      },
    );
  }, [id, show]);

  /** update は読めた中身を変える（編集の結果を先に見せる）。読めていなければ何もしない。 */
  const update = useCallback(
    (change: (data: D) => D) => {
      const current = stateRef.current;
      if (current.kind !== "ready") return;
      show({ kind: "ready", data: change(current.data) });
    },
    [show],
  );

  /** markGone は無くなったこと（編集の窓が受けた 404）を見せる。 */
  const markGone = useCallback(() => {
    reading.current?.abort();
    reading.current = null;
    setMore("idle");
    show({ kind: "gone" });
  }, [show]);

  /** stop は読み込みを捨てる（画面を離れるとき）。 */
  const stop = useCallback(() => {
    reading.current?.abort();
    reading.current = null;
  }, []);

  return { state, stateRef, more, read, loadMore, update, markGone, stop };
}
