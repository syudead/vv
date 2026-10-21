import type {
  FolderListing,
  LibraryItem,
  Progress,
  TagRef,
  VideoSort,
  WatchFilter,
} from "./client";
import { groupsIn, groupsWithMembers } from "./libraryItems";
import { applyTagToTags } from "./tagOrder";

/** Normalize omitted/default sort equally so / and /?sort=addedDesc restore the same snapshot. */
const defaultSort: VideoSort = "addedDesc";

export interface ListKey {
  /** Normalize by trimming surrounding whitespace. */
  query: string;
  /** Omitted equals all. */
  watch?: WatchFilter;
  /** Omitted equals false. */
  playable?: boolean;
  /** Omitted equals false; filtered and unfiltered snapshots remain distinct. */
  favorite?: boolean;
  /** Omitted equals the default sort. */
  sort?: VideoSort;
  /** Included only for random sort. */
  seed?: number;
  /** Folder keys remain distinct from library keys even when other conditions match. */
  folder?: string;
  /** Normalize tag IDs without regard to order. */
  tags?: readonly number[];
  person?: number;
  organization?: number;
  tagKind?: number;
  hasVersions?: boolean;
  addedWithin?: number;
  minDuration?: number;
  maxDuration?: number;
}

export interface ListSnapshot {
  key: string;
  items: LibraryItem[];
  total: number;
  cursor?: string;
  hasMore: boolean;
  scrollY: number;
  /** Retain immediate child folders to avoid another request on return. */
  folderListing?: FolderListing;
  /** Reject snapshots if a different scan has completed since capture; undefined means no scan observed. */
  scanId?: number;
  /** Refresh marked group summaries on restore because their values aggregate member state. */
  staleGroups?: number[];
}

/** Retain one in-memory snapshot only; per-key caches retain thousands of videos, and persistence adds unnecessary serialization. */
let held: (ListSnapshot & { expiresAt?: number }) | undefined;

/** Relative addedWithin filters become stale as time advances; expire snapshots even when their keys match. */
export const ADDED_WITHIN_SNAPSHOT_TTL_MS = 5 * 60 * 1000;

/** NUL separators prevent ambiguous boundaries between search text and other key fields. */
function normalize(key: ListKey): string {
  const sort = key.sort ?? defaultSort;
  const seed = sort === "random" ? String(key.seed ?? "") : "";
  const tags = [...(key.tags ?? [])].sort((a, b) => a - b).join(",");
  const list = [
    key.query.trim(),
    key.watch ?? "all",
    key.playable === true ? "1" : "",
    key.favorite === true ? "1" : "",
    sort,
    seed,
    tags,
    key.person === undefined ? "" : String(key.person),
    key.organization === undefined ? "" : String(key.organization),
    key.tagKind === undefined ? "" : String(key.tagKind),
    key.hasVersions === true ? "1" : "",
    key.addedWithin === undefined ? "" : String(key.addedWithin),
    key.minDuration === undefined ? "" : String(key.minDuration),
    key.maxDuration === undefined ? "" : String(key.maxDuration),
  ].join("\0");
  return key.folder === undefined ? list : `folder\0${key.folder}\0${list}`;
}

let holds = 0;

/** Discard and block snapshots while refreshes settle so navigation cannot preserve stale mutation results; release is idempotent. */
export function holdListSnapshot(): () => void {
  holds += 1;
  held = undefined;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    holds -= 1;
  };
}

export function saveListSnapshot(key: ListKey, value: Omit<ListSnapshot, "key">): void {
  if (holds > 0) {
    held = undefined;
    return;
  }
  held = { key: normalize(key), ...value };
  if (key.addedWithin !== undefined) {
    held.expiresAt = Date.now() + ADDED_WITHIN_SNAPSHOT_TTL_MS;
  }
}

/** Return matching unexpired snapshots without consuming them, supporting StrictMode and repeated back navigation. Missing snapshots restart at page one. */
export function takeListSnapshot(key: ListKey): ListSnapshot | undefined {
  if (held === undefined || held.key !== normalize(key)) return undefined;
  if (held.expiresAt !== undefined && Date.now() >= held.expiresAt) return undefined;
  return held;
}

function markStaleGroups(
  snapshot: ListSnapshot,
  videoIds: readonly number[],
): ListSnapshot {
  return withStaleGroups(snapshot, groupsWithMembers(snapshot.items, videoIds));
}

function withStaleGroups(
  snapshot: ListSnapshot,
  groupIds: readonly number[],
): ListSnapshot {
  if (groupIds.length === 0) return snapshot;
  const merged = new Set([...(snapshot.staleGroups ?? []), ...groupIds]);
  return { ...snapshot, staleGroups: [...merged] };
}

/** Update video progress and mark containing groups stale so returning lists reflect watched state. */
export function applyProgressToListSnapshot(videoId: number, progress: Progress): void {
  if (held === undefined) return;
  let next = markStaleGroups(held, [videoId]);
  if (next.items.some((item) => item.kind === "video" && item.video.id === videoId)) {
    next = {
      ...next,
      items: next.items.map((item) =>
        item.kind === "video" && item.video.id === videoId
          ? { kind: "video", video: { ...item.video, progress } }
          : item,
      ),
    };
  }
  held = next;
}

export function applyVisibilityToListSnapshot(
  videoIds: readonly number[],
  isPublic: boolean,
): void {
  if (held === undefined) return;
  const targets = new Set(videoIds);
  if (
    !held.items.some(
      (item) =>
        item.kind === "video" &&
        targets.has(item.video.id) &&
        item.video.public !== isPublic,
    )
  )
    return;
  held = {
    ...held,
    items: held.items.map((item) =>
      item.kind === "video" && targets.has(item.video.id)
        ? { kind: "video", video: { ...item.video, public: isPublic } }
        : item,
    ),
  };
}

/** Update videos and mark groups for refresh; retain unfavorited rows in the current favorites-only snapshot. */
export function applyFavoritesToListSnapshot(
  videoIds: readonly number[],
  groupIds: readonly number[],
  favorite: boolean,
): void {
  if (held === undefined) return;
  let next = held;
  const targets = new Set(videoIds);
  if (
    next.items.some(
      (item) =>
        item.kind === "video" &&
        targets.has(item.video.id) &&
        item.video.favorite !== favorite,
    )
  ) {
    next = {
      ...next,
      items: next.items.map((item) =>
        item.kind === "video" && targets.has(item.video.id)
          ? { kind: "video", video: { ...item.video, favorite } }
          : item,
      ),
    };
  }
  held = withStaleGroups(next, groupsIn(next.items, groupIds));
}

export function clearListSnapshot(): void {
  held = undefined;
}

/** Apply manual tag changes without removing newly nonmatching rows until reload; mark containing groups stale. */
export function applyTagToListSnapshot(
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
): void {
  if (held === undefined) return;
  const targets = new Set(videoIds);
  let next = markStaleGroups(held, videoIds);
  if (next.items.some((item) => item.kind === "video" && targets.has(item.video.id))) {
    next = {
      ...next,
      items: next.items.map((item) =>
        item.kind === "video" && targets.has(item.video.id)
          ? {
              kind: "video",
              video: {
                ...item.video,
                tags: applyTagToTags(item.video.tags, tag, action),
              },
            }
          : item,
      ),
    };
  }
  held = next;
}
