import { useCallback, useEffect, useRef } from "react";

import type { GroupDetail } from "../api/client";
import { type GroupItem, listGroupItems, readGroupPage } from "../api/groups";
import { readThrough } from "../api/readThrough";
import { subscribeServerEvents } from "../api/serverEvents";
import { type PagedDetailState, usePagedDetail } from "../api/usePagedDetail";
import { appendItems } from "./groupItems";

export interface GroupPageData {
  group: GroupDetail["group"];
  trail: GroupDetail["trail"];
  items: readonly GroupItem[];
  nextCursor?: string;
}

export type GroupPageState = PagedDetailState<GroupPageData>;

const videoRefreshDelayMs = 300;

async function readGroupThrough(
  id: number,
  trail: readonly number[],
  atLeast: number,
  signal: AbortSignal,
  query: string,
): Promise<GroupPageData> {
  const detail = await readGroupPage(id, trail, signal, query);
  const { items, nextCursor } = await readThrough(
    detail,
    (cursor) => listGroupItems(id, cursor, signal, undefined, query),
    appendItems,
    atLeast,
  );
  return { group: detail.group, trail: detail.trail, items, nextCursor };
}

/**
 * Keep visible data during background refreshes, except on 404; discard stale responses. Wait for
 * queued moves before reading, and retry if a move was sent during the read.
 */
export function useGroupPage(id: number | null, trail: readonly number[], query = "") {
  /** Last server-confirmed order, used for rollback. */
  const confirmed = useRef<readonly GroupItem[]>([]);
  /** Serialize moves in drop order and count sent moves. */
  const moves = useRef<Promise<void>>(Promise.resolve());
  const movesSent = useRef(0);
  /** Advance on route changes to prevent queued refreshes from reading after navigation. */
  const epoch = useRef(0);
  /** Retry reads interrupted by moves after the move queue settles. */
  const requery = useRef<() => void>(() => undefined);
  const trailKey = trail.join(",");

  const { state, stateRef, more, read, loadMore, update, markGone, stop } =
    usePagedDetail(
      id,
      {
        read: (groupId: number, atLeast: number, signal: AbortSignal) => {
          return readGroupThrough(
            groupId,
            trailKey === "" ? [] : trailKey.split(",").map(Number),
            atLeast,
            signal,
            query,
          );
        },
        more: (groupId: number, cursor: string, signal: AbortSignal) =>
          listGroupItems(groupId, cursor, signal, undefined, query),
        append: appendItems,
        beforeRead: (quiet: boolean) => {
          const sentBefore = movesSent.current;
          return (data: GroupPageData) => {
            if (quiet && movesSent.current !== sentBefore) {
              // This read may not include moves sent while it was in flight.
              requery.current();
              return false;
            }
            confirmed.current = data.items;
            return true;
          };
        },
        onMore: (items: readonly GroupItem[]) => {
          confirmed.current = appendItems(confirmed.current, items);
        },
      },
      query,
    );

  const reload = useCallback(() => read(false), [read]);
  /** Wait for pending moves before silently refreshing. */
  const refresh = useCallback(() => {
    const ticket = epoch.current;
    void moves.current.then(() => {
      if (ticket === epoch.current) read(true);
    });
  }, [read]);
  useEffect(() => {
    requery.current = refresh;
  }, [refresh]);

  useEffect(() => {
    let videoTimer: ReturnType<typeof setTimeout> | undefined;
    // Subscribe before reading to avoid missing changes after the snapshot.
    const unsubscribe = subscribeServerEvents({
      group: (ids) => {
        if (id !== null && ids.includes(id) && stateRef.current.kind !== "gone") {
          refresh();
        }
      },
      // Coalesce descendant video notifications into one reload of tags, favorites and watch state.
      video: (videoId) => {
        const current = stateRef.current;
        if (current.kind !== "ready" || !current.data.group.videoIds.includes(videoId)) {
          return;
        }
        clearTimeout(videoTimer);
        videoTimer = setTimeout(refresh, videoRefreshDelayMs);
      },
      open: (reconnected) => {
        if (reconnected && stateRef.current.kind === "ready") refresh();
      },
    });
    reload();
    return () => {
      epoch.current += 1;
      unsubscribe();
      clearTimeout(videoTimer);
      stop();
    };
  }, [id, query, refresh, reload, stateRef, stop]);

  return {
    state,
    more,
    reload,
    refresh,
    loadMore,
    update,
    confirmed,
    moves,
    movesSent,
    markGone,
  };
}
