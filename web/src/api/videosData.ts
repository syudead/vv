import type { UiText } from "../i18n";
import type { LibraryGroup, LibraryItem, TagRef, Video } from "./client";
import { itemKey, itemVideos, videoItem } from "./libraryItems";
import { applyTagToTags } from "./tagOrder";

/** Preserve list titles and sizes because folder locations may differ from the detail endpoint's representative location. */
function mergeRefreshed(current: Video, refreshed: Video): Video {
  return { ...current, ...refreshed, title: current.title, sizeBytes: current.sizeBytes };
}

/** Restored data skips the first fetch, avoiding refetching every previously loaded page on return. */
export interface VideosSeed {
  items: LibraryItem[];
  total: number;
  cursor?: string;
  hasMore: boolean;
  /** Refresh groups whose members changed after snapshot capture. */
  staleGroups?: readonly number[];
}

function appendUnique(current: LibraryItem[], next: LibraryItem[]): LibraryItem[] {
  const seen = new Set(current.map(itemKey));
  const added: LibraryItem[] = [];
  for (const item of next) {
    const key = itemKey(item);
    if (seen.has(key)) continue;
    seen.add(key);
    added.push(item);
  }
  return added.length === 0 ? current : [...current, ...added];
}

/** Preserve the original array reference when nothing matches to avoid rerendering. */
function mapVideos(
  items: LibraryItem[],
  match: (video: Video) => boolean,
  update: (video: Video) => Video,
): LibraryItem[] {
  if (!items.some((item) => item.kind === "video" && match(item.video))) return items;
  return items.map((item) =>
    item.kind === "video" && match(item.video) ? videoItem(update(item.video)) : item,
  );
}

/** Visible video rows only, excluding group members. */
export function shownVideoIds(items: readonly LibraryItem[]): number[] {
  return itemVideos(items).map((video) => video.id);
}

export interface VideosData {
  items: LibraryItem[];
  total: number;
  cursor: string | undefined;
  hasMore: boolean;
  inconsistent: boolean;
  /** Missing requested tag IDs; folder listings never send tags and return none. */
  missingTagIds: number[];
}

export type VideosDataAction =
  | { type: "clear" }
  | { type: "stop" }
  | { type: "progress"; videoId: number; progress: NonNullable<Video["progress"]> }
  | {
      type: "tags";
      videoIds: readonly number[];
      tag: TagRef;
      action: "add" | "remove";
    }
  | { type: "visibility"; videoIds: readonly number[]; isPublic: boolean }
  | { type: "favorite"; videoIds: readonly number[]; favorite: boolean }
  | { type: "refresh"; videoId: number; video: Video }
  | { type: "remove"; videoId: number }
  | { type: "refreshGroup"; groupId: number; group: LibraryGroup }
  | { type: "removeGroup"; groupId: number }
  | {
      type: "page";
      page: {
        items: LibraryItem[];
        total: number;
        nextCursor?: string;
        missingTagIds?: number[];
      };
      replace: boolean;
    };

export function videosDataReducer(
  state: VideosData,
  action: VideosDataAction,
): VideosData {
  switch (action.type) {
    case "clear":
      return {
        items: [],
        total: 0,
        cursor: undefined,
        hasMore: true,
        inconsistent: false,
        missingTagIds: [],
      };
    case "stop":
      return state.hasMore ? { ...state, hasMore: false } : state;
    // Overlay changes on video rows; aggregated group values require a group refresh.
    case "progress": {
      const items = mapVideos(
        state.items,
        (video) => video.id === action.videoId,
        (video) => ({ ...video, progress: action.progress }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "tags": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id),
        (video) => ({
          ...video,
          tags: applyTagToTags(video.tags, action.tag, action.action),
        }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "visibility": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id) && video.public !== action.isPublic,
        (video) => ({ ...video, public: action.isPublic }),
      );
      return items === state.items ? state : { ...state, items };
    }
    // Member favorites do not alter group favorites; explicit group toggles refresh separately.
    case "favorite": {
      const targets = new Set(action.videoIds);
      const items = mapVideos(
        state.items,
        (video) => targets.has(video.id) && video.favorite !== action.favorite,
        (video) => ({ ...video, favorite: action.favorite }),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "refresh": {
      const items = mapVideos(
        state.items,
        (video) => video.id === action.videoId,
        (video) => mergeRefreshed(video, action.video),
      );
      return items === state.items ? state : { ...state, items };
    }
    case "remove": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "video" && item.video.id === action.videoId;
      if (!state.items.some(isTarget)) return state;
      return {
        ...state,
        items: state.items.filter((item) => !isTarget(item)),
        total: Math.max(0, state.total - 1),
      };
    }
    case "refreshGroup": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "group" && item.group.id === action.groupId;
      if (!state.items.some(isTarget)) return state;
      return {
        ...state,
        items: state.items.map((item) =>
          isTarget(item) ? { kind: "group", group: action.group } : item,
        ),
      };
    }
    case "removeGroup": {
      const isTarget = (item: LibraryItem) =>
        item.kind === "group" && item.group.id === action.groupId;
      if (!state.items.some(isTarget)) return state;
      // Keep total until the next list read.
      return { ...state, items: state.items.filter((item) => !isTarget(item)) };
    }
    case "page": {
      const missingTagIds = action.page.missingTagIds ?? [];
      if (action.replace) {
        return {
          items: action.page.items,
          total: action.page.total,
          cursor: action.page.nextCursor,
          hasMore: action.page.nextCursor !== undefined,
          inconsistent: false,
          missingTagIds,
        };
      }
      const items = appendUnique(state.items, action.page.items);
      // If the index shrinks between pages, preserve the coherent list and request a restart.
      if (items.length > action.page.total) {
        return { ...state, hasMore: false, inconsistent: true };
      }
      return {
        items,
        total: action.page.total,
        cursor: action.page.nextCursor,
        hasMore: action.page.nextCursor !== undefined,
        inconsistent: false,
        missingTagIds,
      };
    }
  }
}

export interface VideosState {
  items: LibraryItem[];
  total: number;
  cursor: string | undefined;
  hasMore: boolean;
  loading: boolean;
  loadingMore: boolean;
  error: UiText | null;
  notFound: boolean;
  /** Callers report missing tags, refresh their list, and remove these IDs from the URL. */
  missingTagIds: number[];
  loadMore: () => void;
  retryLoadMore: () => void;
  reload: () => void;
  /** Replace loaded pages in place while preserving scroll and selection; transient errors retain current content. */
  refreshInPlace: () => void;
  /** Persist unsettled group IDs, including failed refreshes, so restoration retries them. */
  staleGroups: () => number[];
}
