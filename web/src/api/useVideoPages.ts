import {
  type Dispatch,
  type RefObject,
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
} from "react";

import { errorText, t, type UiText } from "../i18n";
import {
  type FolderRef,
  isAborted,
  type LibraryItem,
  listFolderVideos,
  listLibrary,
  listVideos,
  RequestFailed,
  type TagRef,
} from "./client";
import { favoriteMark, withFavoriteSince, withGroupFavoriteSince } from "./favorites";
import { groupsIn, groupsWithMembers, itemKey, videoItem } from "./libraryItems";
import type { useGroupRefresh } from "./useGroupRefresh";
import type { useItemRefresh } from "./useItemRefresh";
import type { VideosCriteria } from "./videosCriteria";
import { shownVideoIds, type VideosDataAction, type VideosSeed } from "./videosData";
import { visibilityMark, withVisibilitySince } from "./visibility";

interface VideoPagesParams {
  key: string;
  seed: VideosSeed | undefined;
  generation: number;
  inconsistent: boolean;
  cursor: string | undefined;
  hasMore: boolean;
  loading: boolean;
  loadingMore: boolean;
  folderKey: string;
  criteriaRef: RefObject<VideosCriteria>;
  folderRef: RefObject<FolderRef | undefined>;
  libraryRef: RefObject<boolean>;
  itemsRef: RefObject<LibraryItem[]>;
  dispatch: Dispatch<VideosDataAction>;
  setLoading: Dispatch<SetStateAction<boolean>>;
  setLoadingMore: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<UiText | null>>;
  setNotFound: Dispatch<SetStateAction<boolean>>;
  setGeneration: Dispatch<SetStateAction<number>>;
  resyncAttempted: RefObject<boolean>;
  pageLoading: RefObject<boolean>;
  changedWhileLoading: RefObject<Set<number>>;
  groupsChangedWhileLoading: RefObject<Set<number>>;
  progressChangedWhileLoading: RefObject<Set<number>>;
  tagsChangedWhileLoading: RefObject<
    Map<string, { videoId: number; tag: TagRef; action: "add" | "remove" }>
  >;
  groups: ReturnType<typeof useGroupRefresh>;
  itemRefresh: ReturnType<typeof useItemRefresh>;
}

export function useVideoPages({
  key,
  seed,
  generation,
  inconsistent,
  cursor,
  hasMore,
  loading,
  loadingMore,
  folderKey,
  criteriaRef,
  folderRef,
  libraryRef,
  itemsRef,
  dispatch,
  setLoading,
  setLoadingMore,
  setError,
  setNotFound,
  setGeneration,
  resyncAttempted,
  pageLoading,
  changedWhileLoading,
  groupsChangedWhileLoading,
  progressChangedWhileLoading,
  tagsChangedWhileLoading,
  groups: { groupQueue, groupRefreshing, staleGroups, unsettledGroups, refreshGroups },
  itemRefresh: { refreshQueue, refreshing, uncertain, notifyIfIdle, refreshItems },
}: VideoPagesParams) {
  // Retain request identity so obsolete responses cannot overwrite new conditions.
  const inFlight = useRef<AbortController | null>(null);
  // Keep one in-place refresh and coordinate it with page requests.
  const keepInFlight = useRef<AbortController | null>(null);
  const refreshAfterPage = useRef(false);
  // Queue load-more behind refresh so the first page replacement is not lost to cancellation.
  const moreAfterRefresh = useRef(false);
  const cursorRef = useRef(cursor);
  useEffect(() => {
    cursorRef.current = cursor;
  }, [cursor]);
  const fetchPageRef = useRef<
    ((from: string | undefined, replace: boolean, keep?: number) => Promise<void>) | null
  >(null);

  /** In-place refresh refills the loaded count while retaining content, scroll, and selection; transient failures preserve the current list. */
  const fetchPage = useCallback(
    async (from: string | undefined, replace: boolean, keep?: number) => {
      inFlight.current?.abort();
      keepInFlight.current = null;
      if (keep === undefined) moreAfterRefresh.current = false;
      const controller = new AbortController();
      inFlight.current = controller;
      if (keep !== undefined) keepInFlight.current = controller;
      pageLoading.current = true;
      changedWhileLoading.current.clear();
      groupsChangedWhileLoading.current.clear();
      progressChangedWhileLoading.current.clear();
      if (replace) {
        refreshing.current?.abort();
        refreshing.current = null;
        refreshQueue.current.clear();
        groupRefreshing.current?.abort();
        groupRefreshing.current = null;
        groupQueue.current.clear();
        staleGroups.current = [];
        unsettledGroups.current.clear();
        // Clear mutation overlays only when replacing conditions; retain unseen-video changes across later pages.
        tagsChangedWhileLoading.current.clear();
      }

      if (keep !== undefined) {
        // Keep the visible items during an in-place refresh.
      } else if (replace) {
        setLoading(true);
        setError(null);
        setNotFound(false);
      } else {
        setLoadingMore(true);
      }

      let resumeCursor: string | undefined;
      // Count immediately after dispatch because itemsRef still reflects the previous render; queued refreshes must retain appended rows.
      let loadedAfter: number | undefined;
      const mark = visibilityMark();
      const favoriteSince = favoriteMark();
      try {
        const target = folderRef.current;
        const current = criteriaRef.current;
        const params = {
          query: current.query,
          watch: current.watch === "all" ? undefined : current.watch,
          playable: current.playable,
          favorite: current.favorite,
          sort: current.sort,
          seed: current.sort === "random" ? current.seed : undefined,
          cursor: from,
          signal: controller.signal,
        };
        // Folder listings do not accept library-only filters.
        const listOnly = {
          tag: current.tag,
          person: current.person,
          personRole: current.personRole,
          organization: current.organization,
          tagKind: current.tagKind,
          addedWithin: current.addedWithin,
          minDuration: current.minDuration,
          maxDuration: current.maxDuration,
        };
        const fetchOne = (cursorFrom: string | undefined) =>
          target !== undefined
            ? listFolderVideos({
                folder: target,
                scope: current.scope,
                ...params,
                cursor: cursorFrom,
              })
            : libraryRef.current
              ? listLibrary({
                  ...params,
                  ...listOnly,
                  cursor: cursorFrom,
                })
              : listVideos({ ...params, ...listOnly, cursor: cursorFrom });
        const toPage = (fetched: Awaited<ReturnType<typeof fetchOne>>) => ({
          ...fetched,
          items: fetched.items.map((item): LibraryItem => {
            if ("kind" in item) {
              if (item.kind === "smartFolder") return item;
              return item.kind === "video"
                ? videoItem(
                    withFavoriteSince(
                      withVisibilitySince(item.video, mark),
                      favoriteSince,
                    ),
                  )
                : {
                    kind: "group",
                    group: withGroupFavoriteSince(item.group, favoriteSince),
                  };
            }
            return videoItem(
              withFavoriteSince(withVisibilitySince(item, mark), favoriteSince),
            );
          }),
        });
        let page = toPage(await fetchOne(from));
        if (keep !== undefined) {
          // Count unique items while refilling; counting overlaps would stop before the requested row count.
          const seen = new Set<string>();
          const gathered: LibraryItem[] = [];
          const gather = (items: LibraryItem[]) => {
            for (const item of items) {
              const itemId = itemKey(item);
              if (seen.has(itemId)) continue;
              seen.add(itemId);
              gathered.push(item);
            }
          };
          gather(page.items);
          while (gathered.length < keep && page.nextCursor !== undefined) {
            page = toPage(await fetchOne(page.nextCursor));
            gather(page.items);
          }
          page = { ...page, items: gathered };
        }
        // Abort may occur after body parsing, when fetch no longer rejects.
        if (controller.signal.aborted || inFlight.current !== controller) return;
        if (keep !== undefined && page.items.length > page.total) {
          // Keep the coherent current list if pages changed too much to combine.
          resumeCursor = cursorRef.current;
          return;
        }
        if (replace && page.items.length > page.total) {
          if (!resyncAttempted.current) {
            resyncAttempted.current = true;
            setGeneration((value) => value + 1);
          } else {
            dispatch({ type: "clear" });
            dispatch({ type: "stop" });
            setError(t.list.inconsistentPage);
          }
          return;
        }
        if (replace) resyncAttempted.current = false;
        if (keep !== undefined) resumeCursor = page.nextCursor;
        const shownBefore = new Set(shownVideoIds(itemsRef.current));
        // Match reducer deduplication when tracking the count for later refreshes.
        loadedAfter = replace
          ? page.items.length
          : new Set([...itemsRef.current, ...page.items].map(itemKey)).size;
        dispatch({ type: "page", page, replace });
        const changed = shownVideoIds(page.items).filter((id) =>
          changedWhileLoading.current.has(id),
        );
        // Refresh groups whose members changed while this page loaded.
        refreshGroups(
          groupsWithMembers(page.items, [
            ...changedWhileLoading.current,
            ...progressChangedWhileLoading.current,
          ]),
        );
        // Refresh newly encountered changed groups too; deleted groups cannot be corrected by favorite overlays alone.
        refreshGroups(groupsIn(page.items, groupsChangedWhileLoading.current));
        groupsChangedWhileLoading.current.clear();
        changedWhileLoading.current.clear();
        progressChangedWhileLoading.current.clear();
        // Settle only uncertainty confirmed by this page; changes received while loading still require refresh.
        const stillShown = new Set(changed);
        if (!replace) for (const id of shownBefore) stillShown.add(id);
        for (const id of uncertain.current.keys()) {
          if (!stillShown.has(id)) uncertain.current.delete(id);
        }
        if (changed.length > 0) refreshItems(changed);
        // Overlay known tag mutations on newly loaded videos instead of refetching each video.
        // Member tag changes require refreshing their group summary.
        if (tagsChangedWhileLoading.current.size > 0) {
          const pageIds = new Set(shownVideoIds(page.items));
          const memberIds = new Set(
            page.items.flatMap((item) =>
              item.kind === "group" ? item.group.videoIds : [],
            ),
          );
          const changedMembers: number[] = [];
          for (const [tagKey, change] of tagsChangedWhileLoading.current) {
            if (memberIds.has(change.videoId)) {
              changedMembers.push(change.videoId);
              tagsChangedWhileLoading.current.delete(tagKey);
              continue;
            }
            if (!pageIds.has(change.videoId)) continue;
            dispatch({
              type: "tags",
              videoIds: [change.videoId],
              tag: change.tag,
              action: change.action,
            });
            tagsChangedWhileLoading.current.delete(tagKey);
          }
          refreshGroups(groupsWithMembers(page.items, changedMembers));
        }
        setError(null);
        setNotFound(false);
      } catch (failure) {
        // Ignore errors from aborted or obsolete requests so they cannot replace the current list's state.
        if (
          isAborted(failure) ||
          controller.signal.aborted ||
          inFlight.current !== controller
        ) {
          return;
        }
        if (keep !== undefined) {
          resumeCursor = cursorRef.current;
          return;
        }
        if (
          folderRef.current !== undefined &&
          failure instanceof RequestFailed &&
          failure.status === 404
        ) {
          setNotFound(true);
          setError(null);
          dispatch({ type: "stop" });
          return;
        }
        setError(errorText(failure));
        // Clear prior 404 state so a new failure remains visible and retryable.
        setNotFound(false);
        // Remove the pagination sentinel on failure to avoid repeated automatic requests.
        dispatch({ type: "stop" });
      } finally {
        if (!controller.signal.aborted) {
          pageLoading.current = false;
          if (keepInFlight.current === controller) keepInFlight.current = null;
          setLoading(false);
          setLoadingMore(false);
          notifyIfIdle();
          const resumeMore = moreAfterRefresh.current && keep !== undefined;
          if (keep !== undefined) moreAfterRefresh.current = false;
          if (refreshAfterPage.current) {
            refreshAfterPage.current = false;
            void fetchPageRef.current?.(
              undefined,
              true,
              Math.max(loadedAfter ?? itemsRef.current.length, 1),
            );
          } else if (resumeMore && resumeCursor !== undefined) {
            void fetchPageRef.current?.(resumeCursor, false);
          }
        }
      }
    },
    // Dependencies are stable refs/setters; read current criteria at call time and reload through the key effect.
    [
      changedWhileLoading,
      groupsChangedWhileLoading,
      criteriaRef,
      dispatch,
      folderRef,
      groupQueue,
      groupRefreshing,
      itemsRef,
      libraryRef,
      notifyIfIdle,
      pageLoading,
      progressChangedWhileLoading,
      refreshGroups,
      refreshItems,
      refreshQueue,
      refreshing,
      resyncAttempted,
      setError,
      setGeneration,
      setLoading,
      setLoadingMore,
      setNotFound,
      staleGroups,
      tagsChangedWhileLoading,
      uncertain,
      unsettledGroups,
    ],
  );

  useEffect(() => {
    fetchPageRef.current = fetchPage;
  }, [fetchPage]);

  // Track restored content by key rather than consuming a flag so StrictMode replay preserves restoration.
  //
  const seeded = useRef(seed === undefined ? null : { key, folderKey, generation: 0 });

  // When pages cannot be combined, retain coherent content and restart with a new generation.
  useEffect(() => {
    if (!inconsistent) return;
    if (!resyncAttempted.current) {
      resyncAttempted.current = true;
      setGeneration((value) => value + 1);
      return;
    }
    dispatch({ type: "clear" });
    dispatch({ type: "stop" });
    setError(t.list.inconsistentPage);
  }, [dispatch, inconsistent, resyncAttempted, setError, setGeneration]);

  // Cursors belong to their conditions/folder and cannot survive a key change.
  //
  useEffect(() => {
    const held = seeded.current;
    if (
      held !== null &&
      held.key === key &&
      held.folderKey === folderKey &&
      held.generation === generation
    ) {
      // Restored lists still need cleanup to abort load-more requests when leaving.
      return () => inFlight.current?.abort();
    }
    seeded.current = null;

    dispatch({ type: "clear" });
    void fetchPage(undefined, true);

    return () => inFlight.current?.abort();
  }, [dispatch, fetchPage, folderKey, generation, key]);

  const loadMore = useCallback(() => {
    if (loading || loadingMore || !hasMore || cursor === undefined) {
      return;
    }
    if (keepInFlight.current !== null) {
      moreAfterRefresh.current = true;
      return;
    }
    void fetchPage(cursor, false);
  }, [cursor, fetchPage, hasMore, loading, loadingMore]);

  const retryLoadMore = useCallback(() => {
    if (loading || loadingMore || cursor === undefined) return;
    if (keepInFlight.current !== null) {
      moreAfterRefresh.current = true;
      return;
    }
    void fetchPage(cursor, false);
  }, [cursor, fetchPage, loading, loadingMore]);

  const reload = useCallback(() => {
    resyncAttempted.current = false;
    setGeneration((value) => value + 1);
  }, [resyncAttempted, setGeneration]);

  const refreshInPlace = useCallback(() => {
    if (pageLoading.current && keepInFlight.current === null) {
      refreshAfterPage.current = true;
      return;
    }
    void fetchPage(undefined, true, Math.max(itemsRef.current.length, 1));
  }, [fetchPage, itemsRef, pageLoading]);

  return { loadMore, retryLoadMore, reload, refreshInPlace };
}
