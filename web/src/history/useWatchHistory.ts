import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { listWatchHistory, type WatchHistoryEntry } from "../api/history";
import { type HistoryCriteria, historyCriteriaKey } from "./historyCriteria";

export type MoreState =
  { kind: "idle" } | { kind: "loading" } | { kind: "failed"; error: unknown };

export type HistoryState =
  | { kind: "loading" }
  | { kind: "failed" }
  | {
      kind: "ready";
      items: WatchHistoryEntry[];
      nextCursor?: string;
      more: MoreState;
    };

function conditionsOf(criteria: HistoryCriteria) {
  return {
    ...(criteria.watch === "all" ? {} : { watch: criteria.watch }),
    ...(criteria.query === "" ? {} : { query: criteria.query }),
    ...(criteria.date === undefined ? {} : { date: criteria.date }),
  };
}

export interface WatchHistory {
  state: HistoryState;
  /** No-op during loading or first-page refresh, or without a continuation cursor. */
  loadMore: () => void;
  retry: () => void;
  /** Keep rows during a stale-data reload; failures retain them and call onReloadFailed. */
  reload: () => void;
  /**
   * Return the next focus ID, falling back to the previous record; fetch immediately if all loaded
   * rows disappear.
   */
  remove: (id: number) => number | null;
  /** Discard in-flight responses after clearing history. */
  clear: () => void;
}

/**
 * Keep pagination conditions stable and include the browser timezone for date filtering. Remember
 * deleted IDs so older in-flight responses cannot restore them.
 */
export function useWatchHistory(
  criteria: HistoryCriteria,
  onReloadFailed: (error: unknown) => void,
): WatchHistory {
  const [state, setState] = useState<HistoryState>({ kind: "loading" });
  // Async responses need the latest rows without waiting for rendering.
  const stateRef = useRef(state);
  const show = useCallback((next: HistoryState) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const onReloadFailedRef = useRef(onReloadFailed);
  useEffect(() => {
    onReloadFailedRef.current = onReloadFailed;
  }, [onReloadFailed]);

  // Paginate with the displayed list's conditions, not a newer pending query.
  const criteriaKey = historyCriteriaKey(criteria);
  const latestCriteria = useRef(criteria);
  useEffect(() => {
    latestCriteria.current = criteria;
  });
  const shownCriteria = useRef(criteria);

  // Advance on first-page reload or clear to invalidate stale responses.
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

  // Load directly when all rows disappear: there is no final row for IntersectionObserver.
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
      shownCriteria.current = latestCriteria.current;
      listWatchHistory({
        ...conditionsOf(shownCriteria.current),
        signal: request.signal,
      }).then(
        (page) => {
          request.done();
          if (gen !== generation.current) return;
          readingFirst.current = false;
          // Continue from the response cursor even if every returned entry was deleted while loading.
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
            // Replace the row array to reattach the observer after refresh; a still-visible final row otherwise emits no new event. If no rows remain, fetch directly.
            fill({ ...now, items: [...now.items] });
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
    listWatchHistory({
      ...conditionsOf(shownCriteria.current),
      cursor,
      signal: request.signal,
    }).then(
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
  }, [begin, readFirst, criteriaKey]);

  return { state, loadMore, retry, reload, remove, clear };
}
