import { type Dispatch, type RefObject, useCallback, useRef } from "react";

import { getGroup, isAborted, type LibraryItem, RequestFailed } from "./client";
import { groupsWithMembers, shownGroupIds } from "./libraryItems";
import type { VideosDataAction, VideosSeed } from "./videosData";

export function useGroupRefresh(
  seed: VideosSeed | undefined,
  itemsRef: RefObject<LibraryItem[]>,
  dispatch: Dispatch<VideosDataAction>,
) {
  // Refresh individual group summaries after member changes; retain content during loading and remove 404s.
  // Serialize group refreshes and schedule one follow-up when the same group changes during its request.
  const groupQueue = useRef(new Set<number>());
  const groupRefreshing = useRef<AbortController | null>(null);
  const staleGroups = useRef<readonly number[]>(seed?.staleGroups ?? []);
  // Persist unsettled groups in snapshots so interrupted or failed refreshes retry after navigation.
  const unsettledGroups = useRef(new Set<number>(seed?.staleGroups ?? []));
  const drainGroupQueue = useCallback(async () => {
    if (groupRefreshing.current !== null) return;
    const controller = new AbortController();
    groupRefreshing.current = controller;
    try {
      for (const groupId of groupQueue.current) {
        groupQueue.current.delete(groupId);
        try {
          const { group } = await getGroup(groupId, controller.signal);
          // Never overlay stale refresh results on a list loaded with new conditions.
          if (controller.signal.aborted) return;
          // Keep the stale marker if another change arrived during this refresh.
          if (!groupQueue.current.has(groupId)) unsettledGroups.current.delete(groupId);
          dispatch({ type: "refreshGroup", groupId, group });
        } catch (failure) {
          if (isAborted(failure) || controller.signal.aborted) return;
          // Retain transient failures as unsettled so restoring the snapshot retries them.
          if (failure instanceof RequestFailed && failure.status === 404) {
            unsettledGroups.current.delete(groupId);
            dispatch({ type: "removeGroup", groupId });
          }
        }
      }
    } finally {
      if (groupRefreshing.current === controller) groupRefreshing.current = null;
    }
  }, [dispatch]);
  const refreshGroups = useCallback(
    (groupIds: Iterable<number>) => {
      let queued = false;
      for (const groupId of groupIds) {
        groupQueue.current.add(groupId);
        unsettledGroups.current.add(groupId);
        queued = true;
      }
      if (queued) void drainGroupQueue();
    },
    [drainGroupQueue],
  );
  const refreshGroupsWith = useCallback(
    (videoIds: Iterable<number>) => {
      refreshGroups(groupsWithMembers(itemsRef.current, videoIds));
    },
    [itemsRef, refreshGroups],
  );
  const unsettledGroupRefs = useCallback((): number[] => {
    const pending = unsettledGroups.current;
    if (pending.size === 0) return [];
    return shownGroupIds(itemsRef.current).filter((id) => pending.has(id));
  }, [itemsRef]);

  return {
    groupQueue,
    groupRefreshing,
    staleGroups,
    unsettledGroups,
    refreshGroups,
    refreshGroupsWith,
    unsettledGroupRefs,
  };
}
