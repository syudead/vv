import { type Dispatch, type RefObject, useCallback, useRef } from "react";

import { getGroup, isAborted, type LibraryItem, RequestFailed } from "./client";
import { groupsWithMembers, shownGroupIds } from "./libraryItems";
import type { VideosDataAction, VideosSeed } from "./videosData";

/** useGroupRefresh は useVideos のグループの項目の取り直しである。 */
export function useGroupRefresh(
  seed: VideosSeed | undefined,
  itemsRef: RefObject<LibraryItem[]>,
  dispatch: Dispatch<VideosDataAction>,
) {
  // グループの項目は、メンバーの再生位置・タグ・`video` と `group` イベントの変化で
  // `GET /api/groups/{id}` から1件ずつ取り直して差し替える。
  // 取り直しの間は項目を変えず、404 ならその項目を外す。
  // 取り直しは1件ずつ順に行い、同じグループを重ねて取りに行かない（動画の
  // 取り直しの refreshQueue と同じ形）。取り直しの途中に同じグループが変われば、
  // 終わった後にもう一度取る。
  const groupQueue = useRef(new Set<number>());
  const groupRefreshing = useRef<AbortController | null>(null);
  // staleGroups は、控えから戻った一覧のうち、控えの後にメンバーが変わったグループの id である。
  const staleGroups = useRef<readonly number[]>(seed?.staleGroups ?? []);
  // unsettledGroups は、取り直しを求めたがまだ差し替えも除外もしていないグループである
  // （待ち・取り直し中・一時的な失敗）。一覧を離れるときの控えに印として残し、戻った
  // ときに取り直す。取り直しの途中で動画を開くと要求は打ち切られるので、印を残さないと
  // 古いグループがそのまま復元される（Devin の指摘、PR 357）。
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
          // 条件を変えて読み直した後に届いた古い取り直しは、新しい一覧に重ねない。
          if (controller.signal.aborted) return;
          // 取り直しの途中に同じグループがまた変わっていれば、次の取り直しまで印を残す。
          if (!groupQueue.current.has(groupId)) unsettledGroups.current.delete(groupId);
          dispatch({ type: "refreshGroup", groupId, group });
        } catch (failure) {
          if (isAborted(failure) || controller.signal.aborted) return;
          // 今はグループが無い（解かれた等）か、見せる動画が無い。何も伝えずに外す。
          // 一時的な失敗は、その1件だけ諦める（次の変化か読み直しで直る）。
          // 一時的な失敗のグループは unsettledGroups に残し、控えの印で戻ったときに取り直す。
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
  // refreshGroupsWith は `videoIds` のどれかをメンバーに持つ表示中のグループを取り直す。
  const refreshGroupsWith = useCallback(
    (videoIds: Iterable<number>) => {
      refreshGroups(groupsWithMembers(itemsRef.current, videoIds));
    },
    [itemsRef, refreshGroups],
  );
  // unsettledGroupRefs は控えに残す取り直しの印である（表示中のグループだけ）。
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
