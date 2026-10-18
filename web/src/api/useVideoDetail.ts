import { useCallback, useEffect, useRef, useState } from "react";

import { useAudience } from "../auth/audience";
import { errorText, type UiText } from "../i18n";
import {
  getRelatedVideos,
  getVideo,
  isAborted,
  RequestFailed,
  type RelatedVideos,
  type Video,
} from "./client";
import {
  favoriteMark,
  favoriteReadMark,
  noteFavoriteOf,
  subscribeFavorites,
  subscribeFavoritesStale,
  updateFavorites,
  withFavoriteSince,
} from "./favorites";
import { subscribeServerEvents } from "./serverEvents";
import {
  subscribeVideoVisibility,
  subscribeVideoVisibilityStale,
  visibilityMark,
  withVisibilitySince,
} from "./visibility";

export type VideoDetailState =
  | { kind: "loading"; id: number }
  | { kind: "ready"; id: number; video: Video }
  | { kind: "missing"; id: number }
  | { kind: "failed"; id: number; reason: UiText };

/** Probe failures are terminal: preview jobs never start, so their pending state must not keep processing active. */
export function isProcessing(video: Video): boolean {
  if (video.probeState === "pending") return true;
  if (video.probeState !== "done") return false;
  return (
    video.thumbnailState === "pending" ||
    video.seekThumbnailState === "pending" ||
    video.previewState === "pending"
  );
}

export interface VideoReadContext {
  group?: number;
  smartFolder?: number;
}

/** Preserve opened group/folder context because mutation responses ignore it and return default membership. */
function withGroupOf(
  saved: Video,
  current: Video | undefined,
  opened: VideoReadContext,
): Video {
  if (current === undefined) return saved;
  if (opened.group === undefined && opened.smartFolder === undefined) return saved;
  const video = { ...saved };
  if (current.group === undefined) delete video.group;
  else video.group = current.group;
  if (current.smartFolder === undefined) delete video.smartFolder;
  else video.smartFolder = current.smartFolder;
  return video;
}

/** Opened groups win while they contain the video; otherwise the lowest-ID direct group wins. Refresh when events can change that choice. */
function groupEventConcerns(
  ids: readonly number[],
  shown: number | undefined,
  group?: number,
): boolean {
  if (shown === undefined) return true;
  if (group !== undefined && ids.includes(group)) return true;
  if (shown === group) return false;
  return ids.some((changed) => changed <= shown);
}

let loadsStarted = 0;

/** Capture before mutations so replace can distinguish earlier reads from later refreshes. */
export type DetailMark = { load: number; visibility: number; favorite: number };

export function detailMark(): DetailMark {
  return { load: loadsStarted, visibility: visibilityMark(), favorite: favoriteMark() };
}

/** Keep omitted metadata for unrelated mutations; refetch after unbundling because the video's own metadata replaces the bundle's. */
export type ReplaceWorkInfo = "keep" | "refetch";

function withWorkInfoOf(saved: Video, current: Video | undefined): Video {
  if (saved.workInfo !== undefined || current?.workInfo === undefined) return saved;
  return { ...saved, workInfo: current.workInfo };
}

/** Preserve mediaInfo omitted by mutation responses; only detail reads include it. */
function withMediaInfoOf(saved: Video, current: Video | undefined): Video {
  if (saved.mediaInfo !== undefined || current?.mediaInfo === undefined) return saved;
  return { ...saved, mediaInfo: current.mediaInfo };
}

/** Refresh from events, retaining data on transient failure and rejecting stale responses with detail marks. Preserve playback across context changes and confirm local favorites from refreshed data. */
export function useVideoDetail(
  id: number,
  group?: number,
  initial?: Video,
  smartFolder?: number,
): {
  state: VideoDetailState;
  refresh: () => Promise<void>;
  replace: (video: Video, mark: DetailMark, workInfo: ReplaceWorkInfo) => void;
  setFavorite: (favorite: boolean) => Promise<void>;
} {
  const [state, setState] = useState<VideoDetailState>(() =>
    initial !== undefined && initial.id === id
      ? { kind: "ready", id, video: initial }
      : { kind: "loading", id },
  );
  // Seed from the editor's saved response without flashing stale data or a loading state.
  const seed = useRef(initial);
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  const replaceRef = useRef<
    (video: Video, mark: DetailMark, workInfo: ReplaceWorkInfo) => void
  >(() => undefined);
  const setFavoriteRef = useRef<(favorite: boolean) => Promise<void>>(() =>
    Promise.resolve(),
  );
  // Only owners receive video events; guests may receive group events.
  const owner = useAudience() === "owner";

  // Update context before the fetch effect so video changes use the new context immediately.
  const contextRef = useRef({ id, group, smartFolder });
  useEffect(() => {
    const previous = contextRef.current;
    contextRef.current = { id, group, smartFolder };
    if (
      previous.id === id &&
      (previous.group !== group || previous.smartFolder !== smartFolder)
    )
      void refreshRef.current();
  }, [id, group, smartFolder]);

  useEffect(() => {
    // Keep the seed across StrictMode replay, discarding it only when the video changes.
    const seeded = seed.current?.id === id ? seed.current : undefined;
    if (seeded === undefined) seed.current = undefined;
    setState(
      seeded === undefined
        ? { kind: "loading", id }
        : { kind: "ready", id, video: seeded },
    );
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "missing", id });
      refreshRef.current = () => Promise.resolve();
      replaceRef.current = () => undefined;
      setFavoriteRef.current = () => Promise.resolve();
      return;
    }

    let alive = true;
    let controller: AbortController | null = null;
    let started = 0;
    let current: Video | undefined = seeded;
    let currentLoad = 0;
    let currentRead = 0;
    const waiters: (() => void)[] = [];
    let ownFavorites = 0;

    const settle = () => {
      for (const resolve of waiters.splice(0)) resolve();
    };

    const load = async () => {
      controller?.abort();
      const mine = new AbortController();
      controller = mine;
      loadsStarted += 1;
      started = loadsStarted;
      // Preserve confirmed visibility/favorite changes over older reads; no server event would repair a rollback.
      const mark = visibilityMark();
      const favorite = favoriteMark();
      const read = favoriteReadMark();
      const loadStarted = started;
      try {
        const video = withFavoriteSince(
          withVisibilitySince(
            await getVideo(
              id,
              mine.signal,
              contextRef.current.group,
              contextRef.current.smartFolder,
            ),
            mark,
          ),
          favorite,
        );
        if (!alive || controller !== mine) return;
        current = video;
        currentLoad = loadStarted;
        currentRead = read;
        setState({ kind: "ready", id, video });
        settle();
      } catch (failure) {
        if (!alive || controller !== mine || isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          current = undefined;
          setState({ kind: "missing", id });
          settle();
          return;
        }
        if (current === undefined) {
          setState({ kind: "failed", id, reason: errorText(failure) });
        }
        settle();
      }
    };

    refreshRef.current = () =>
      new Promise<void>((resolve) => {
        if (!alive) {
          resolve();
          return;
        }
        waiters.push(resolve);
        void load();
      });

    replaceRef.current = (saved, mark, workInfo) => {
      if (!alive || saved.id !== id) return;
      if (controller !== null && started <= mark.load) {
        controller.abort();
        controller = null;
        settle();
      }
      const grouped = withMediaInfoOf(
        withGroupOf(saved, current, contextRef.current),
        current,
      );
      const video = withFavoriteSince(
        withVisibilitySince(
          workInfo === "keep" ? withWorkInfoOf(grouped, current) : grouped,
          mark.visibility,
        ),
        mark.favorite,
      );
      current = video;
      // Mutation responses have no read timestamp, so do not propagate them to the representative.
      currentLoad = 0;
      setState({ kind: "ready", id, video });
      // Refresh after replacing data that may have overwritten an already-completed event refresh.
      if (workInfo === "refetch") void load();
    };

    setFavoriteRef.current = async (favorite) => {
      ownFavorites += 1;
      try {
        await updateFavorites([id], [], favorite);
      } finally {
        ownFavorites -= 1;
      }
      // Propagate only post-mutation reads; failed refreshes leave current holding the pre-mutation value.
      const after = loadsStarted;
      await refreshRef.current();
      // Versions share favorites; update the visible representative and snapshot from the refreshed value.
      const representative = current?.versions?.representativeId;
      if (
        alive &&
        current !== undefined &&
        currentLoad > after &&
        current.favorite !== undefined &&
        representative !== undefined &&
        representative !== id
      ) {
        noteFavoriteOf([representative], current.favorite, currentRead);
      }
    };

    const unsubscribeVisibility = subscribeVideoVisibility((videoIds, isPublic) => {
      if (current === undefined || !videoIds.includes(id)) return;
      if (current.public === isPublic) return;
      current = { ...current, public: isPublic };
      setState({ kind: "ready", id, video: current });
    });
    // Partial application cannot identify changed videos; reload this video's authoritative state.
    // Detail pages take no list snapshots, so they need not report refresh settlement.
    const unsubscribeStale = subscribeVideoVisibilityStale((videoIds) => {
      if (videoIds.includes(id)) void load();
      return undefined;
    });

    // Do not add favorite fields absent from guest responses.
    // Local favorite changes are confirmed by setFavorite's refresh, not by this overlay.
    const unsubscribeFavorites = subscribeFavorites((change) => {
      if (ownFavorites > 0) return;
      if (current === undefined || !change.videoIds.includes(id)) return;
      if (current.favorite === undefined || current.favorite === change.favorite) return;
      current = { ...current, favorite: change.favorite };
      setState({ kind: "ready", id, video: current });
    });
    const unsubscribeFavoritesStale = subscribeFavoritesStale((videoIds) => {
      if (videoIds.includes(id)) void load();
      return undefined;
    });

    //
    // Smart-folder context may have fallen back to a group, so every group event can affect navigation.
    const onGroup = (ids: readonly number[]) => {
      const opened = contextRef.current;
      if (
        opened.smartFolder !== undefined ||
        groupEventConcerns(ids, current?.group?.id, opened.group)
      )
        void load();
    };
    //
    const onSmartFolder = (ids: readonly number[]) => {
      const opened = contextRef.current.smartFolder;
      if (opened !== undefined && ids.includes(opened)) void load();
    };
    // Subscribe before fetching to avoid missed changes; guests subscribe only to group events.
    const unsubscribe = owner
      ? subscribeServerEvents({
          video: (changed) => {
            if (changed === id) void load();
          },
          group: onGroup,
          smartFolder: onSmartFolder,
          // Reconnection requires refresh because events may have been missed.
          // Avoid duplicate initial reads unless processing could have completed before the event connection opened.
          open: (reconnected) => {
            if (reconnected || (current !== undefined && isProcessing(current)))
              void load();
          },
        })
      : subscribeServerEvents({
          group: onGroup,
          smartFolder: onSmartFolder,
          open: (reconnected) => {
            if (reconnected) void load();
          },
        });
    void load();
    return () => {
      alive = false;
      controller?.abort();
      unsubscribe();
      unsubscribeVisibility();
      unsubscribeStale();
      unsubscribeFavorites();
      unsubscribeFavoritesStale();
      settle();
    };
  }, [id, owner]);

  const refresh = useCallback(() => refreshRef.current(), []);
  const replace = useCallback(
    (video: Video, mark: DetailMark, workInfo: ReplaceWorkInfo) =>
      replaceRef.current(video, mark, workInfo),
    [],
  );
  const setFavorite = useCallback(
    (favorite: boolean) => setFavoriteRef.current(favorite),
    [],
  );
  return { state, refresh, replace, setFavorite };
}

export type RelatedState =
  | { kind: "loading"; id: number }
  | {
      kind: "ready";
      id: number;
      related: RelatedVideos;
      more?: "idle" | "loading" | "failed";
      /** Increment first-page generations to reset appended member windows and their pending requests. */
      generation?: number;
    }
  | { kind: "failed"; id: number };

/** Paginate with deduplication and shared context; patch saved titles/thumbnails into member rows without reloading. */
export function useRelatedVideos(
  id: number,
  group?: number,
  smartFolder?: number,
): {
  state: RelatedState;
  retry: () => void;
  loadMore: () => void;
  rename: (video: Video) => void;
  rethumb: (video: Video) => void;
} {
  const [state, setState] = useState<RelatedState>({ kind: "loading", id });
  const [attempt, setAttempt] = useState(0);
  const stateRef = useRef(state);
  useEffect(() => {
    stateRef.current = state;
  }, [state]);
  const moreAbort = useRef<AbortController | null>(null);
  const contextRef = useRef({ group, smartFolder });
  useEffect(() => {
    contextRef.current = { group, smartFolder };
  }, [group, smartFolder]);

  useEffect(() => {
    setState({ kind: "loading", id });
    if (!Number.isSafeInteger(id) || id < 1) {
      setState({ kind: "ready", id, related: { items: [] }, more: "idle" });
      return;
    }
    let controller = new AbortController();
    // After first display, quiet refreshes retain the current list even on failure.
    let shownOnce = false;
    let generation = 0;
    const show = (related: RelatedVideos) => {
      shownOnce = true;
      generation += 1;
      setState({ kind: "ready", id, related, more: "idle", generation });
    };
    const load = async () => {
      const mine = controller;
      try {
        const related = await getRelatedVideos(
          id,
          mine.signal,
          undefined,
          group,
          smartFolder,
        );
        if (mine.signal.aborted) return;
        show(related);
      } catch (failure) {
        if (mine.signal.aborted || isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          show({ items: [] });
          return;
        }
        if (!shownOnce) setState({ kind: "failed", id });
      }
    };
    // Restart at page one and clear aborted load-more state so pagination remains retryable if refresh fails.
    const reload = () => {
      controller.abort();
      if (moreAbort.current !== null) {
        moreAbort.current.abort();
        moreAbort.current = null;
        setState((previous) =>
          previous.kind === "ready" && previous.more === "loading"
            ? { ...previous, more: "idle" }
            : previous,
        );
      }
      controller = new AbortController();
      void load();
    };
    // Refresh for group changes or the opened smart folder, matching useVideoDetail's fallback handling.
    const unsubscribe = subscribeServerEvents({
      group: (ids) => {
        const current = stateRef.current;
        const shown = current.kind === "ready" ? current.related.group?.id : undefined;
        if (smartFolder !== undefined || groupEventConcerns(ids, shown, group)) reload();
      },
      smartFolder: (ids) => {
        if (smartFolder !== undefined && ids.includes(smartFolder)) reload();
      },
      open: (reconnected) => {
        if (reconnected) reload();
      },
    });
    void load();
    return () => {
      unsubscribe();
      controller.abort();
      moreAbort.current?.abort();
      moreAbort.current = null;
    };
  }, [attempt, id, group, smartFolder]);

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (current.kind !== "ready" || current.more === "loading") return;
    const cursor = current.related.nextCursor;
    if (cursor === undefined) return;
    const controller = new AbortController();
    moreAbort.current = controller;
    const loading = { ...current, more: "loading" as const };
    stateRef.current = loading;
    setState(loading);
    const opened = contextRef.current;
    getRelatedVideos(
      current.id,
      controller.signal,
      cursor,
      opened.group,
      opened.smartFolder,
    ).then(
      (page) => {
        if (controller.signal.aborted) return;
        setState((previous) => {
          if (previous.kind !== "ready" || previous.id !== current.id) return previous;
          const seen = new Set(previous.related.items.map((item) => item.id));
          const added = page.items.filter((item) => !seen.has(item.id));
          const related: RelatedVideos = {
            ...previous.related,
            items: [...previous.related.items, ...added],
          };
          delete related.nextCursor;
          if (page.nextCursor !== undefined) related.nextCursor = page.nextCursor;
          return { ...previous, related, more: "idle" };
        });
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        setState((previous) =>
          previous.kind === "ready" && previous.id === current.id
            ? { ...previous, more: "failed" }
            : previous,
        );
      },
    );
  }, []);

  const retry = useCallback(() => setAttempt((value) => value + 1), []);
  const patchMember = useCallback((video: Video, patch: (item: Video) => Video) => {
    setState((previous) => {
      if (previous.kind !== "ready" || previous.id !== video.id) return previous;
      const group = previous.related.group;
      if (group === undefined || !group.items.some((item) => item.id === video.id)) {
        return previous;
      }
      const items = group.items.map((item) =>
        item.id === video.id ? patch(item) : item,
      );
      return {
        ...previous,
        related: { ...previous.related, group: { ...group, items } },
      };
    });
  }, []);
  const rename = useCallback(
    (video: Video) =>
      patchMember(video, (item) => {
        const renamed: Video = { ...item, title: video.title };
        delete renamed.displayName;
        if (video.fileTitle !== undefined) renamed.fileTitle = video.fileTitle;
        if (video.displayName !== undefined) renamed.displayName = video.displayName;
        return renamed;
      }),
    [patchMember],
  );
  const rethumb = useCallback(
    (video: Video) =>
      patchMember(video, (item) => {
        const updated: Video = { ...item };
        delete updated.thumbnailUrl;
        delete updated.thumbnailPositionMs;
        if (video.thumbnailUrl !== undefined) updated.thumbnailUrl = video.thumbnailUrl;
        if (video.thumbnailPositionMs !== undefined) {
          updated.thumbnailPositionMs = video.thumbnailPositionMs;
        }
        return updated;
      }),
    [patchMember],
  );
  return { state, retry, loadMore, rename, rethumb };
}
