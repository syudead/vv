import { type Dispatch, type RefObject, useCallback, useRef } from "react";

import {
  type FolderRef,
  getFolderGroup,
  isAborted,
  type LibraryItem,
  RequestFailed,
} from "./client";
import { folderRefKey, groupRef, groupsWithMembers } from "./libraryItems";
import type { VideosDataAction, VideosSeed } from "./videosData";

/** useGroupRefresh は useVideos のグループの項目の取り直しである。 */
export function useGroupRefresh(
  seed: VideosSeed | undefined,
  itemsRef: RefObject<LibraryItem[]>,
  dispatch: Dispatch<VideosDataAction>,
) {
  // グループの項目は、メンバーの再生位置・タグ・`video` イベントの変化で
  // `GET /api/folders/{rootId}/group` から1件ずつ取り直して差し替える
  // 取り直しの間は項目を変えず、404 ならその項目を外す。
  // 取り直しは1件ずつ順に行い、同じグループを重ねて取りに行かない（動画の
  // 取り直しの refreshQueue と同じ形）。取り直しの途中に同じグループが変われば、
  // 終わった後にもう一度取る。
  const groupQueue = useRef(new Map<string, FolderRef>());
  const groupRefreshing = useRef<AbortController | null>(null);
  // staleGroups は、控えから戻った一覧のうち、控えの後にメンバーが変わったグループである。
  const staleGroups = useRef<readonly FolderRef[]>(seed?.staleGroups ?? []);
  // unsettledGroups は、取り直しを求めたがまだ差し替えも除外もしていないグループである
  // （待ち・取り直し中・一時的な失敗）。一覧を離れるときの控えに印として残し、戻った
  // ときに取り直す。取り直しの途中で動画を開くと要求は打ち切られるので、印を残さないと
  // 古いグループがそのまま復元される（Devin の指摘、PR 357）。
  const unsettledGroups = useRef(
    new Map((seed?.staleGroups ?? []).map((folder) => [folderRefKey(folder), folder])),
  );
  const drainGroupQueue = useCallback(async () => {
    if (groupRefreshing.current !== null) return;
    const controller = new AbortController();
    groupRefreshing.current = controller;
    try {
      for (const [groupKey, folder] of groupQueue.current) {
        groupQueue.current.delete(groupKey);
        try {
          const group = await getFolderGroup(folder, controller.signal);
          // 条件を変えて読み直した後に届いた古い取り直しは、新しい一覧に重ねない。
          if (controller.signal.aborted) return;
          // 取り直しの途中に同じグループがまた変わっていれば、次の取り直しまで印を残す。
          if (!groupQueue.current.has(groupKey)) unsettledGroups.current.delete(groupKey);
          dispatch({ type: "refreshGroup", folderKey: groupKey, group });
        } catch (failure) {
          if (isAborted(failure) || controller.signal.aborted) return;
          // 今はグループでない（例外で単体に戻った等）か、フォルダが無い。何も伝えずに
          // 外す。一時的な失敗は、その1件だけ諦める（次の変化か読み直しで直る）。
          // 一時的な失敗のグループは unsettledGroups に残し、控えの印で戻ったときに取り直す。
          if (failure instanceof RequestFailed && failure.status === 404) {
            unsettledGroups.current.delete(groupKey);
            dispatch({ type: "removeGroup", folderKey: groupKey });
          }
        }
      }
    } finally {
      if (groupRefreshing.current === controller) groupRefreshing.current = null;
    }
  }, [dispatch]);
  const refreshGroups = useCallback(
    (folders: Iterable<FolderRef>) => {
      let queued = false;
      for (const folder of folders) {
        const groupKey = folderRefKey(folder);
        groupQueue.current.set(groupKey, folder);
        unsettledGroups.current.set(groupKey, folder);
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
  const unsettledGroupRefs = useCallback((): FolderRef[] => {
    const pending = unsettledGroups.current;
    if (pending.size === 0) return [];
    return itemsRef.current.flatMap((item) => {
      if (item.kind !== "group") return [];
      const folder = groupRef(item.group);
      return pending.has(folderRefKey(folder)) ? [folder] : [];
    });
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
