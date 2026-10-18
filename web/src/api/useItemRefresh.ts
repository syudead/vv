import { type Dispatch, type RefObject, useCallback, useRef } from "react";

import { getVideo, isAborted, type LibraryItem, RequestFailed } from "./client";
import { itemVideos } from "./libraryItems";
import { isProcessing } from "./useVideoDetail";
import { shownVideoIds, type VideosDataAction } from "./videosData";
import { favoriteMark, withFavoriteSince } from "./favorites";
import { visibilityMark, withVisibilitySince } from "./visibility";

export type UncertainReason = "visibility" | "favorite";

export function useItemRefresh(
  pageLoading: RefObject<boolean>,
  itemsRef: RefObject<LibraryItem[]>,
  dispatch: Dispatch<VideosDataAction>,
  // Resync from the start when only the server can determine which collapsed items belong in the list.
  resync: () => void,
) {
  // Refresh videos individually and serially to preserve loaded pages and scroll position.
  const refreshQueue = useRef(new Set<number>());
  const refreshing = useRef<AbortController | null>(null);
  // Wait for item/page refreshes to settle partial mutations; release waiters on unmount too.
  const idleWaiters = useRef<(() => void)[]>([]);
  // Uncertain mutation results must remain unresolved through transient failures, preventing stale snapshots.
  //
  // Compare notice generations so a read started before a mutation cannot settle its later uncertainty.
  //
  // Track visibility and favorite uncertainty independently; confirming one cannot settle the other.
  const uncertain = useRef(new Map<number, Map<UncertainReason, number>>());
  const staleNotices = useRef(0);
  const markUncertain = useCallback(
    (id: number, reason: UncertainReason, notice: number) => {
      const reasons = uncertain.current.get(id);
      if (reasons === undefined) uncertain.current.set(id, new Map([[reason, notice]]));
      else reasons.set(reason, notice);
    },
    [],
  );
  // Settle only reasons whose latest notice predates this refresh.
  const settleUncertain = useCallback((id: number, started: number) => {
    const reasons = uncertain.current.get(id);
    if (reasons === undefined) return;
    for (const [reason, notice] of reasons) {
      if (notice <= started) reasons.delete(reason);
    }
    if (reasons.size === 0) uncertain.current.delete(id);
  }, []);
  const settleUncertainReason = useCallback(
    (ids: Iterable<number>, reason: UncertainReason) => {
      for (const id of ids) {
        const reasons = uncertain.current.get(id);
        if (reasons === undefined) continue;
        reasons.delete(reason);
        if (reasons.size === 0) uncertain.current.delete(id);
      }
    },
    [],
  );
  const notifyIfIdle = useCallback(() => {
    if (
      pageLoading.current ||
      refreshing.current !== null ||
      refreshQueue.current.size > 0 ||
      uncertain.current.size > 0
    ) {
      return;
    }
    for (const resolve of idleWaiters.current.splice(0)) resolve();
  }, [pageLoading]);
  const drainRefreshQueue = useCallback(async () => {
    if (refreshing.current !== null) return;
    const controller = new AbortController();
    refreshing.current = controller;
    try {
      for (const id of refreshQueue.current) {
        refreshQueue.current.delete(id);
        const started = staleNotices.current;
        try {
          const mark = visibilityMark();
          const favoriteSince = favoriteMark();
          const refreshed = withFavoriteSince(
            withVisibilitySince(await getVideo(id, controller.signal), mark),
            favoriteSince,
          );
          // Do not apply old refresh results after list conditions change.
          if (controller.signal.aborted) return;
          // Remove a nonrepresentative if its representative is shown; otherwise resync because only the server knows whether that representative matches filters.
          const representativeId = refreshed.versions?.representativeId;
          if (representativeId !== undefined && representativeId !== id) {
            settleUncertain(id, started);
            if (shownVideoIds(itemsRef.current).includes(representativeId)) {
              dispatch({ type: "remove", videoId: id });
              continue;
            }
            resync();
            return;
          }
          settleUncertain(id, started);
          dispatch({ type: "refresh", videoId: id, video: refreshed });
        } catch (failure) {
          if (isAborted(failure)) return;
          // Remove missing videos; transient failures wait for a later event or scan refresh.
          // Failed refreshes retain uncertainty.
          if (failure instanceof RequestFailed && failure.status === 404) {
            settleUncertain(id, started);
            dispatch({ type: "remove", videoId: id });
          }
        }
      }
    } finally {
      if (refreshing.current === controller) refreshing.current = null;
      notifyIfIdle();
    }
  }, [dispatch, itemsRef, notifyIfIdle, resync, settleUncertain]);
  const refreshItems = useCallback(
    (ids: Iterable<number>) => {
      for (const id of ids) refreshQueue.current.add(id);
      void drainRefreshQueue();
    },
    [drainRefreshQueue],
  );
  const refreshProcessingItems = useCallback(() => {
    refreshItems(
      itemVideos(itemsRef.current)
        .filter((video) => isProcessing(video))
        .map((video) => video.id),
    );
  }, [itemsRef, refreshItems]);

  return {
    refreshQueue,
    refreshing,
    idleWaiters,
    uncertain,
    staleNotices,
    markUncertain,
    settleUncertainReason,
    notifyIfIdle,
    refreshItems,
    refreshProcessingItems,
  };
}
