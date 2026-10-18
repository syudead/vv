import { apiFetch, MAX_PAGE_SIZE, PAGE_SIZE, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

export type PlaylistSummary = components["schemas"]["PlaylistSummary"];
type ApiPlaylist = components["schemas"]["Playlist"];
type ApiPlaylistItem = components["schemas"]["PlaylistItem"];
export type PlaylistItem = ApiPlaylistItem;
export type Playlist = ApiPlaylist;
export type PlaylistItemsAdded = components["schemas"]["PlaylistItemsAdded"];
export type PlaylistPlayback = components["schemas"]["PlaylistPlayback"];
export type PlaylistStep = components["schemas"]["PlaylistStep"];

const jsonHeaders = { "Content-Type": "application/json" };

function playlistPath(id: number): string {
  return `/api/playlists/${String(id)}`;
}

function playlistItemPath(id: number, itemId: number): string {
  return `${playlistPath(id)}/items/${String(itemId)}`;
}

export type PlaylistList = components["schemas"]["PlaylistList"];

export interface ListPlaylistsParams {
  /** Opaque nextCursor from the previous response. */
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

export function listPlaylists({
  cursor,
  limit = PAGE_SIZE,
  signal,
}: ListPlaylistsParams = {}): Promise<PlaylistList> {
  const params = new URLSearchParams();
  if (cursor !== undefined) params.set("cursor", cursor);
  params.set("limit", String(limit));
  return request<PlaylistList>(`/api/playlists?${params.toString()}`, { signal });
}

export const MAX_LIST_RESTARTS = 3;

/** Restart pagination when version changes because updated playlists can move before the cursor. Fail after bounded retries rather than return an incomplete list. */
export async function listAllPlaylists(signal?: AbortSignal): Promise<PlaylistSummary[]> {
  for (let restarts = 0; ; restarts++) {
    const items: PlaylistSummary[] = [];
    let cursor: string | undefined;
    let version: string | undefined;
    let changed = false;
    do {
      const page = await listPlaylists({ cursor, limit: MAX_PAGE_SIZE, signal });
      version ??= page.version;
      if (page.version !== version) {
        changed = true;
        break;
      }
      items.push(...page.items);
      cursor = page.nextCursor;
    } while (cursor !== undefined);
    if (!changed) return items;
    if (restarts >= MAX_LIST_RESTARTS) {
      throw new Error("The playlists kept changing while they were being listed.");
    }
  }
}

export function createPlaylist(
  name: string,
  signal?: AbortSignal,
): Promise<PlaylistSummary> {
  return request<PlaylistSummary>("/api/playlists", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ name }),
    signal,
  });
}

/** Preserve server item positions, including smart-folder items. */
export function getPlaylist(id: number, signal?: AbortSignal): Promise<Playlist> {
  return request<Playlist>(playlistPath(id), { signal });
}

export function renamePlaylist(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<PlaylistSummary> {
  return request<PlaylistSummary>(playlistPath(id), {
    method: "PATCH",
    headers: jsonHeaders,
    body: JSON.stringify({ name }),
    signal,
  });
}

export async function deletePlaylist(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(playlistPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Append videos, groups, then smart folders; count existing items without adding duplicates. */
export function addToPlaylist(
  id: number,
  videoIds: readonly number[],
  groupIds: readonly number[],
  signal?: AbortSignal,
  smartFolderIds: readonly number[] = [],
): Promise<PlaylistItemsAdded> {
  return request<PlaylistItemsAdded>(`${playlistPath(id)}/items`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({
      videoIds: Array.from(videoIds),
      groupIds: Array.from(groupIds),
      ...(smartFolderIds.length === 0
        ? {}
        : { smartFolderIds: Array.from(smartFolderIds) }),
    }),
    signal,
  });
}

export async function movePlaylistItem(
  id: number,
  itemId: number,
  afterItemId: number | null,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(`${playlistItemPath(id, itemId)}/move`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ afterItemId }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Remove only the playlist item, preserving the underlying entity, favorites, and progress. */
export async function removePlaylistItem(
  id: number,
  itemId: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(playlistItemPath(id, itemId), {
    method: "DELETE",
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Read position/next from current order; missing playlists/items return 404 so callers drop playlist context. */
export function getPlaylistPlayback(
  playlistId: number,
  itemId: number,
  videoId: number,
  signal?: AbortSignal,
): Promise<PlaylistPlayback> {
  const params = new URLSearchParams({ item: String(itemId), video: String(videoId) });
  return request<PlaylistPlayback>(
    `/api/playlists/${String(playlistId)}/playback?${params.toString()}`,
    { signal },
  );
}
