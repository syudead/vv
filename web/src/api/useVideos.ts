import { useCallback, useEffect, useReducer, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import type { UiText } from "../i18n";
import type { TagRef } from "./client";
import { subscribeFavorites, subscribeFavoritesStale } from "./favorites";
import { folderRefKey, groupsIn, shownGroupIds } from "./libraryItems";
import { subscribeProgress } from "./progressEvents";
import { subscribeServerEvents } from "./serverEvents";
import { useGroupRefresh } from "./useGroupRefresh";
import { type UncertainReason, useItemRefresh } from "./useItemRefresh";
import { useVideoPages } from "./useVideoPages";
import { subscribeVideoTags } from "./videoTagsEvents";
import {
  criteriaKey,
  listsSmartFolders,
  type VideosCriteria,
  type VideosSource,
} from "./videosCriteria";
import {
  shownVideoIds,
  type VideosData,
  videosDataReducer,
  type VideosSeed,
  type VideosState,
} from "./videosData";
import { subscribeVideoVisibility, subscribeVideoVisibilityStale } from "./visibility";

export type { VideosCriteria, VideosSource } from "./videosCriteria";
export type { VideosSeed, VideosState } from "./videosData";

/** Apply filtering on the server and load one page initially. A restored snapshot skips page one while its conditions remain unchanged. */
export function useVideos(
  criteria: VideosCriteria,
  restored?: VideosSeed,
  source: VideosSource = "videos",
): VideosState {
  const folder = typeof source === "string" ? undefined : source;
  const library = source === "library";
  // Compare criteria by value so fresh caller objects do not trigger reloads.
  const key = criteriaKey(criteria);
  const criteriaRef = useRef(criteria);
  criteriaRef.current = criteria;
  const seed = restored;
  // Compare folders by value for the same reason.
  const folderKey =
    folder === undefined ? (library ? "library" : "") : folderRefKey(folder);
  const folderRef = useRef(folder);
  folderRef.current = folder;
  const libraryRef = useRef(library);
  libraryRef.current = library;
  const [{ items, total, cursor, hasMore, inconsistent, missingTagIds }, dispatch] =
    useReducer(videosDataReducer, seed, (initial): VideosData => ({
      items: initial?.items ?? [],
      total: initial?.total ?? 0,
      cursor: initial?.cursor,
      hasMore: initial?.hasMore ?? true,
      inconsistent: false,
      missingTagIds: [],
    }));
  const [loading, setLoading] = useState(seed === undefined);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [generation, setGeneration] = useState(0);
  const resyncAttempted = useRef(false);

  useEffect(() => {
    resyncAttempted.current = false;
  }, [folderKey, key]);

  const itemsRef = useRef(items);
  itemsRef.current = items;

  const groups = useGroupRefresh(seed, itemsRef, dispatch);
  const {
    groupQueue,
    groupRefreshing,
    staleGroups,
    refreshGroups,
    refreshGroupsWith,
    unsettledGroupRefs,
  } = groups;

  // Events arriving during page loads may describe newer state than the pending response.
  const pageLoading = useRef(false);

  // Newly loaded groups may predate progress saves; refresh matching summaries after applying the page.
  const progressChangedWhileLoading = useRef(new Set<number>());

  // Exit saves can arrive after restoration; apply them to the mounted list too.
  useEffect(
    () =>
      subscribeProgress((videoId, progress) => {
        if (pageLoading.current) progressChangedWhileLoading.current.add(videoId);
        dispatch({ type: "progress", videoId, progress });
        refreshGroupsWith([videoId]);
      }),
    [refreshGroupsWith],
  );

  // Retain the latest tag mutation per video/tag across pages until the matching video loads.
  // Bound retained changes by evicting the oldest entries.
  const tagsChangedWhileLoading = useRef(
    new Map<string, { videoId: number; tag: TagRef; action: "add" | "remove" }>(),
  );
  const maxTagsChangedWhileLoading = 500;

  // Apply mutations without immediately removing rows that no longer match filters.
  useEffect(
    () =>
      subscribeVideoTags((videoIds, tag, action) => {
        if (pageLoading.current) {
          const map = tagsChangedWhileLoading.current;
          for (const videoId of videoIds) {
            map.set(`${String(videoId)}\0${String(tag.id)}`, { videoId, tag, action });
          }
          // Map iteration preserves insertion order for oldest-first eviction.
          while (map.size > maxTagsChangedWhileLoading) {
            const oldest = map.keys().next();
            if (oldest.done) break;
            map.delete(oldest.value);
          }
        }
        dispatch({ type: "tags", videoIds, tag, action });
        refreshGroupsWith(videoIds);
      }),
    [refreshGroupsWith],
  );

  const resync = useCallback(() => {
    resyncAttempted.current = false;
    setGeneration((value) => value + 1);
  }, []);
  const itemRefresh = useItemRefresh(pageLoading, itemsRef, dispatch, resync);
  const {
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
  } = itemRefresh;

  const changedWhileLoading = useRef(new Set<number>());
  const groupsChangedWhileLoading = useRef(new Set<number>());

  // Partial mutation results require refresh; block snapshots until requests settle or the list unmounts.
  // Transient failures keep uncertainty unresolved so navigation cannot preserve stale data.
  useEffect(() => {
    const waiters = idleWaiters.current;
    const pending = uncertain.current;
    // Track uncertainty per reason; confirming favorites cannot settle visibility and vice versa.
    //
    const onStale = (reason: UncertainReason) => (videoIds: readonly number[]) => {
      const targets = new Set(videoIds);
      staleNotices.current += 1;
      const notice = staleNotices.current;
      if (pageLoading.current) {
        // Pending pages may predate the toggle; refresh affected arrivals and settle absent targets afterward.
        for (const id of targets) {
          changedWhileLoading.current.add(id);
          markUncertain(id, reason, notice);
        }
      }
      const shown = shownVideoIds(itemsRef.current).filter((id) => targets.has(id));
      if (!pageLoading.current && shown.length === 0) return undefined;
      for (const id of shown) markUncertain(id, reason, notice);
      const settled = new Promise<void>((resolve) => {
        waiters.push(resolve);
      });
      refreshItems(shown);
      notifyIfIdle();
      return settled;
    };
    const unsubscribeVisibility = subscribeVideoVisibilityStale(onStale("visibility"));
    const unsubscribeFavorites = subscribeFavoritesStale(onStale("favorite"));
    return () => {
      unsubscribeVisibility();
      unsubscribeFavorites();
      pending.clear();
      for (const resolve of waiters.splice(0)) resolve();
    };
  }, [idleWaiters, markUncertain, notifyIfIdle, refreshItems, staleNotices, uncertain]);

  // Overlay visibility changes on pending pages and item refreshes without dropping records at a count limit.
  //
  // Fully applied results settle visibility uncertainty only; sequence guards already exclude older mutations.
  useEffect(
    () =>
      subscribeVideoVisibility((videoIds, isPublic) => {
        settleUncertainReason(videoIds, "visibility");
        dispatch({ type: "visibility", videoIds, isPublic });
        notifyIfIdle();
      }),
    [notifyIfIdle, settleUncertainReason],
  );

  // Member favorites do not alter group favorites; refresh only explicitly toggled groups.
  // Do not remove or reorder current rows even for favorite filters/sorts.
  //
  // Confirmed favorites settle only favorite uncertainty, preserving any visibility uncertainty.
  // Remember changed groups during page loads, including zero-application groups that may need removal on 404.
  useEffect(
    () =>
      subscribeFavorites(({ videoIds, groupIds, favorite }) => {
        if (videoIds.length > 0) {
          settleUncertainReason(videoIds, "favorite");
          dispatch({ type: "favorite", videoIds, favorite });
          notifyIfIdle();
        }
        if (groupIds.length === 0) return;
        if (pageLoading.current) {
          for (const id of groupIds) groupsChangedWhileLoading.current.add(id);
        }
        refreshGroups(groupsIn(itemsRef.current, groupIds));
      }),
    [notifyIfIdle, refreshGroups, settleUncertainReason],
  );

  // Guest subscriptions include only group and smart-folder events; video and scan events are owner-only.
  const owner = useAudience() === "owner";
  const refreshInPlaceRef = useRef<() => void>(() => undefined);
  useEffect(() => {
    const queue = refreshQueue.current;
    // Group structure changes can add or remove top-level cards, so refreshing visible groups alone is insufficient.
    const group = (ids: readonly number[]) => {
      if (libraryRef.current) {
        refreshInPlaceRef.current();
        return;
      }
      if (pageLoading.current) {
        for (const id of ids) groupsChangedWhileLoading.current.add(id);
      }
      refreshGroups(groupsIn(itemsRef.current, ids));
    };
    // Smart-folder events do not distinguish structure from matching changes; refresh loaded pages while the provider refreshes card values.
    const smartFolder = () => {
      const current = criteriaRef.current;
      if (libraryRef.current && listsSmartFolders(current)) {
        refreshInPlaceRef.current();
      }
    };
    // Reconnect refreshes library structure to recover changes missed while disconnected.
    const regroup = () => {
      if (libraryRef.current) refreshInPlaceRef.current();
      else refreshGroups(shownGroupIds(itemsRef.current));
    };
    const unsubscribe = owner
      ? subscribeServerEvents({
          video: (id) => {
            // Retain events even for visible videos during page loading, or a later stale page could undo their refresh.
            if (pageLoading.current) changedWhileLoading.current.add(id);
            if (shownVideoIds(itemsRef.current).includes(id)) refreshItems([id]);
            refreshGroupsWith([id]);
          },
          group,
          smartFolder,
          open: (reconnected) => {
            if (reconnected) {
              refreshItems(shownVideoIds(itemsRef.current));
              regroup();
            } else {
              refreshProcessingItems();
            }
          },
        })
      : subscribeServerEvents({
          group,
          smartFolder,
          open: (reconnected) => {
            if (reconnected) regroup();
          },
        });
    // Restored processing items and stale groups may have changed while another page was open.
    refreshProcessingItems();
    refreshGroups(groupsIn(itemsRef.current, staleGroups.current));
    const groups = groupQueue.current;
    return () => {
      unsubscribe();
      refreshing.current?.abort();
      refreshing.current = null;
      queue.clear();
      groupRefreshing.current?.abort();
      groupRefreshing.current = null;
      groups.clear();
    };
  }, [
    groupQueue,
    groupRefreshing,
    owner,
    refreshGroups,
    refreshGroupsWith,
    refreshItems,
    refreshProcessingItems,
    refreshQueue,
    refreshing,
    staleGroups,
  ]);

  const { loadMore, retryLoadMore, reload, refreshInPlace } = useVideoPages({
    key,
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
    groups,
    itemRefresh,
    seed,
    generation,
    inconsistent,
    cursor,
    hasMore,
    loading,
    loadingMore,
  });
  refreshInPlaceRef.current = refreshInPlace;

  return {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    notFound,
    missingTagIds,
    loadMore,
    retryLoadMore,
    reload,
    refreshInPlace,
    staleGroups: unsettledGroupRefs,
  };
}
