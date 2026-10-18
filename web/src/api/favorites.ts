import { type LibraryItemOf, request } from "./client";
import type { components } from "./gen/openapi";
import { applyFavoritesToListSnapshot, holdListSnapshot } from "./listSnapshot";
import { smartFolderField } from "./smartFolderChanges";

export type FavoritesResponse = components["schemas"]["FavoritesResponse"];

/** Publish favorite changes locally because no server event exists; refresh group summaries whose values remain server-owned. */
export interface FavoritesChange {
  videoIds: readonly number[];
  groupIds: readonly number[];
  favorite: boolean;
}

type Listener = (change: FavoritesChange) => void;

const listeners = new Set<Listener>();

/** Partial application cannot identify changed videos; refresh them instead of assuming success locally. */
type StaleListener = (videoIds: readonly number[]) => Promise<void> | undefined;

const staleListeners = new Set<StaleListener>();

let sent = 0;

/** All mutations through this sequence are settled because sends are serialized. */
let settledThrough = 0;

/** Latest applied sequence per video, preventing stale response rollback. */
const applied = new Map<number, number>();

/** Overlay changes on GETs started before mutations so stale favorite values cannot overwrite confirmed edits. */
let changes = 0;
const latestVideos = new Map<number, { favorite: boolean; change: number }>();
const latestGroups = new Map<number, { favorite: boolean; change: number }>();

let tail: Promise<void> | undefined;

function record(
  videoIds: readonly number[],
  uncertainIds: readonly number[],
  groupIds: readonly number[],
  groupsCertain: boolean,
  favorite: boolean,
  sequence: number,
): void {
  const isFresh = (videoId: number) => {
    if (sequence <= (applied.get(videoId) ?? 0)) return false;
    applied.set(videoId, sequence);
    return true;
  };
  const fresh = videoIds.filter(isFresh);
  const uncertain = uncertainIds.filter(isFresh);
  changes += 1;
  for (const videoId of fresh) latestVideos.set(videoId, { favorite, change: changes });
  for (const videoId of uncertain) latestVideos.delete(videoId);
  for (const groupId of groupIds) {
    if (groupsCertain) latestGroups.set(groupId, { favorite, change: changes });
    else latestGroups.delete(groupId);
  }
  if (fresh.length > 0 || groupIds.length > 0) {
    applyFavoritesToListSnapshot(fresh, groupIds, favorite);
    for (const listener of listeners) listener({ videoIds: fresh, groupIds, favorite });
  }
  if (uncertain.length > 0) {
    // Block snapshots until uncertain results have been refreshed.
    const release = holdListSnapshot();
    for (const listener of staleListeners) {
      const refetch = listener(uncertain);
      if (refetch === undefined) continue;
      const releaseOne = holdListSnapshot();
      void refetch.then(releaseOne, releaseOne);
    }
    release();
  }
}

export function subscribeFavorites(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Subscribers must settle their refresh promises before snapshots are allowed again. */
export function subscribeFavoritesStale(listener: StaleListener): () => void {
  staleListeners.add(listener);
  return () => {
    staleListeners.delete(listener);
  };
}

/** Propagate shared version favorites to the representative. Use the GET's favoriteReadMark so older reads cannot overwrite later mutations. */
export function noteFavoriteOf(
  videoIds: readonly number[],
  favorite: boolean,
  sequence: number,
): void {
  if (videoIds.length === 0) return;
  record(Array.from(new Set(videoIds)), [], [], true, favorite, sequence);
}

/** Capture settled mutations before starting a GET; pass this mark to noteFavoriteOf. */
export function favoriteReadMark(): number {
  return settledThrough;
}

/** Capture applied changes before starting a GET. */
export function favoriteMark(): number {
  return changes;
}

/** Overlay favorite changes applied after mark. */
export function withFavoriteSince<T extends { id: number; favorite?: boolean }>(
  video: T,
  mark: number,
): T {
  const change = latestVideos.get(video.id);
  if (change === undefined || change.change <= mark) return video;
  if (video.favorite === change.favorite) return video;
  return { ...video, favorite: change.favorite };
}

export function withGroupFavoriteSince<T extends { id: number; favorite?: boolean }>(
  group: T,
  mark: number,
): T {
  const change = latestGroups.get(group.id);
  if (change === undefined || change.change <= mark) return group;
  if (group.favorite === change.favorite) return group;
  return { ...group, favorite: change.favorite };
}

export function withItemFavoriteSince<
  T extends LibraryItemOf<{ id: number; favorite?: boolean }>,
>(item: T, mark: number): T {
  if (item.kind === "video") {
    return { ...item, video: withFavoriteSince(item.video, mark) };
  }
  if (item.kind === "group") {
    return { ...item, group: withGroupFavoriteSince(item.group, mark) };
  }
  return item;
}

export interface UpdateFavoritesOptions {
  /** The local overlay covers videos/groups only; bulk callers announce smart folders with noteSmartFolderChanges. */
  smartFolderIds?: readonly number[];
  signal?: AbortSignal;
}

/** Serialize favorite updates and notify subscribers only after success. */
export function updateFavorites(
  videoIds: readonly number[],
  groupIds: readonly number[],
  favorite: boolean,
  { smartFolderIds = [], signal }: UpdateFavoritesOptions = {},
): Promise<FavoritesResponse> {
  sent += 1;
  const sequence = sent;
  const send = async () => {
    try {
      return await put();
    } finally {
      settledThrough = Math.max(settledThrough, sequence);
    }
  };
  const put = async () => {
    const result = await request<FavoritesResponse>("/api/favorites", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        videoIds: Array.from(videoIds),
        groupIds: Array.from(groupIds),
        ...smartFolderField(smartFolderIds),
        favorite,
      }),
      signal,
    });
    const unique = Array.from(new Set(videoIds));
    const uniqueGroups = Array.from(new Set(groupIds));
    const videosCertain = result.appliedVideos >= unique.length;
    record(
      videosCertain ? unique : [],
      videosCertain ? [] : unique,
      uniqueGroups,
      result.appliedGroups >= uniqueGroups.length,
      favorite,
      sequence,
    );
    return result;
  };
  // Send immediately when idle so pending state begins with the click.
  const run = tail === undefined ? send() : tail.then(send);
  const settled = run.then(
    () => undefined,
    () => undefined,
  );
  tail = settled;
  void settled.then(() => {
    if (tail === settled) tail = undefined;
  });
  return run;
}
