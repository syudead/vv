import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { listWatchHistory, type WatchHistoryEntry } from "../api/history";

// 視聴履歴の一覧の読み込み（specs/043-watch-history/ui-design.md「Paging」「States」）。

export type MoreState =
  { kind: "idle" } | { kind: "loading" } | { kind: "failed"; error: unknown };

export type HistoryState =
  | { kind: "loading" }
  | { kind: "failed" }
  | {
      kind: "ready";
      items: WatchHistoryEntry[];
      /** 続きのページの鍵。無ければもう古い件は無い。 */
      nextCursor?: string;
      more: MoreState;
    };

export interface WatchHistory {
  state: HistoryState;
  /** 次のページを読む。読んでいる間、続きが無いとき、最初のページを読み直している間は何もしない。 */
  loadMore: () => void;
  /** 最初のページから読み直す。読んでいる間は読み込み中の表示にする。 */
  retry: () => void;
  /**
   * 今の行を残したまま最初のページから読み直し、届いたら差し替える（削除の 404。R-6）。
   * 失敗したら行を残して `onReloadFailed` を呼ぶ。
   */
  reload: () => void;
  /**
   * 消えた件を一覧から外し、次にフォーカスを置く件（次の件、無ければ前の件）の id を返す。
   * 行が尽きても続きがあれば、すぐに次のページを読む。
   */
  remove: (id: number) => number | null;
  /** 全件を消した後の空の一覧にする。読んでいる要求は捨てる。 */
  clear: () => void;
}

/**
 * useWatchHistory は視聴履歴を新しい順にページごとに読む。消した件の id を覚えておき、
 * 消す前に始まった読み込みの応答にあっても一覧へ戻さない。
 */
export function useWatchHistory(onReloadFailed: (error: unknown) => void): WatchHistory {
  const [state, setState] = useState<HistoryState>({ kind: "loading" });
  // 応答は後から届くので、描画を待たずに今の一覧を読めるようにする。
  const stateRef = useRef(state);
  const show = useCallback((next: HistoryState) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const onReloadFailedRef = useRef(onReloadFailed);
  useEffect(() => {
    onReloadFailedRef.current = onReloadFailed;
  }, [onReloadFailed]);

  // 最初のページを読み直すたび、全件を消すたびに増やし、古い応答を捨てる。
  const generation = useRef(0);
  const controllers = useRef(new Set<AbortController>());
  const readingFirst = useRef(false);
  const removed = useRef(new Set<number>());

  const begin = useCallback(() => {
    generation.current += 1;
    for (const controller of controllers.current) controller.abort();
    controllers.current.clear();
    readingFirst.current = false;
    return generation.current;
  }, []);

  const track = useCallback(() => {
    const controller = new AbortController();
    controllers.current.add(controller);
    return {
      signal: controller.signal,
      done: () => controllers.current.delete(controller),
    };
  }, []);

  // 読み込んだ行が尽きても古い件が残っていれば、すぐに続きを読む（「Paging」）。行が
  // 無いと最後の行を見る IntersectionObserver が働かないので、ここで読む。
  const loadMoreRef = useRef(() => {});
  const fill = useCallback(
    (next: HistoryState) => {
      show(next);
      if (
        next.kind === "ready" &&
        next.items.length === 0 &&
        next.nextCursor !== undefined
      )
        loadMoreRef.current();
    },
    [show],
  );

  const kept = useCallback(
    (items: readonly WatchHistoryEntry[]) =>
      items.filter((item) => !removed.current.has(item.id)),
    [],
  );

  const readFirst = useCallback(
    (keepRows: boolean) => {
      const current = stateRef.current;
      const keeping = keepRows && current.kind === "ready";
      const gen = begin();
      readingFirst.current = true;
      if (!keeping) show({ kind: "loading" });
      else if (current.more.kind === "loading")
        show({ ...current, more: { kind: "idle" } });
      const request = track();
      listWatchHistory({ signal: request.signal }).then(
        (page) => {
          request.done();
          if (gen !== generation.current) return;
          readingFirst.current = false;
          // 読み直しの間に消した件が応答の全部でも、応答の鍵から続きを読む。
          fill({
            kind: "ready",
            items: kept(page.items),
            nextCursor: page.nextCursor,
            more: { kind: "idle" },
          });
        },
        (error: unknown) => {
          request.done();
          if (isAborted(error) || gen !== generation.current) return;
          readingFirst.current = false;
          const now = stateRef.current;
          if (keeping && now.kind === "ready") {
            // 行は残す。読み直しの間は続きの読み込みを断っているので、最後の行が見えたままでも
            // 見張りの知らせは二度と来ない。行の配列を新しくして見張りを付け直させ、見えて
            // いればすぐ続きを読ませる。
            show({ ...now, items: [...now.items] });
            onReloadFailedRef.current(error);
            return;
          }
          show({ kind: "failed" });
        },
      );
    },
    [begin, fill, kept, show, track],
  );

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (
      current.kind !== "ready" ||
      current.nextCursor === undefined ||
      current.more.kind === "loading" ||
      readingFirst.current
    )
      return;
    const gen = generation.current;
    const cursor = current.nextCursor;
    show({ ...current, more: { kind: "loading" } });
    const request = track();
    listWatchHistory({ cursor, signal: request.signal }).then(
      (page) => {
        request.done();
        const now = stateRef.current;
        if (gen !== generation.current || now.kind !== "ready") return;
        const known = new Set(now.items.map((item) => item.id));
        const added = kept(page.items).filter((item) => !known.has(item.id));
        fill({
          kind: "ready",
          items: [...now.items, ...added],
          nextCursor: page.nextCursor,
          more: { kind: "idle" },
        });
      },
      (error: unknown) => {
        request.done();
        const now = stateRef.current;
        if (isAborted(error) || gen !== generation.current || now.kind !== "ready")
          return;
        show({ ...now, more: { kind: "failed", error } });
      },
    );
  }, [fill, kept, show, track]);
  useEffect(() => {
    loadMoreRef.current = loadMore;
  }, [loadMore]);

  const remove = useCallback(
    (id: number): number | null => {
      removed.current.add(id);
      const current = stateRef.current;
      if (current.kind !== "ready") return null;
      const index = current.items.findIndex((item) => item.id === id);
      if (index < 0) return null;
      const items = current.items.filter((_, position) => position !== index);
      fill({ ...current, items });
      const next = items[index] ?? items[index - 1];
      return next === undefined ? null : next.id;
    },
    [fill],
  );

  const clear = useCallback(() => {
    begin();
    show({ kind: "ready", items: [], more: { kind: "idle" } });
  }, [begin, show]);

  const retry = useCallback(() => readFirst(false), [readFirst]);
  const reload = useCallback(() => readFirst(true), [readFirst]);

  useEffect(() => {
    readFirst(false);
    return () => {
      begin();
    };
  }, [begin, readFirst]);

  return { state, loadMore, retry, reload, remove, clear };
}
