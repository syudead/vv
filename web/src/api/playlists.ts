import { apiFetch, type FolderRef, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// プレイリストの画面 API（specs/044-playlists/contracts/screen-api.md「Client use」）。
// 型は api/openapi.yaml からの生成物を使う。どの経路も所有者だけである。

export type PlaylistSummary = components["schemas"]["PlaylistSummary"];
export type Playlist = components["schemas"]["Playlist"];
export type PlaylistItem = components["schemas"]["PlaylistItem"];
export type PlaylistItemsAdded = components["schemas"]["PlaylistItemsAdded"];

const jsonHeaders = { "Content-Type": "application/json" };

function playlistPath(id: number): string {
  return `/api/playlists/${String(id)}`;
}

function playlistItemPath(id: number, itemId: number): string {
  return `${playlistPath(id)}/items/${String(itemId)}`;
}

/** listPlaylists はプレイリストを更新日時の新しい順に取得する（GET /api/playlists）。 */
export async function listPlaylists(signal?: AbortSignal): Promise<PlaylistSummary[]> {
  const list = await request<components["schemas"]["PlaylistList"]>("/api/playlists", {
    signal,
  });
  return list.items;
}

/**
 * createPlaylist は空のプレイリストを作る（POST /api/playlists）。名前は前後の空白を取り除いて
 * 1〜100 文字でなければ 400 `invalid_request` の RequestFailed になる。
 */
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

/** getPlaylist はプレイリスト 1 つと見せる項目を位置の順に取得する（GET /api/playlists/{id}）。 */
export function getPlaylist(id: number, signal?: AbortSignal): Promise<Playlist> {
  return request<Playlist>(playlistPath(id), { signal });
}

/** renamePlaylist はプレイリストの名前を変え、トリムした名前の要約を返す（PATCH /api/playlists/{id}）。 */
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

/** deletePlaylist はプレイリストを消す（DELETE /api/playlists/{id}）。無ければ 404 の RequestFailed。 */
export async function deletePlaylist(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(playlistPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/**
 * addToPlaylist は `videoIds` の動画と `folders` のグループを、この順でプレイリストの末尾に足す
 * （POST /api/playlists/{id}/items）。既にあるものは `existing` に数えられ、何も変わらない。
 */
export function addToPlaylist(
  id: number,
  videoIds: readonly number[],
  folders: readonly FolderRef[],
  signal?: AbortSignal,
): Promise<PlaylistItemsAdded> {
  return request<PlaylistItemsAdded>(`${playlistPath(id)}/items`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({
      videoIds: Array.from(videoIds),
      folders: folders.map((folder) => ({ rootId: folder.rootId, path: folder.path })),
    }),
    signal,
  });
}

/**
 * movePlaylistItem は項目 `itemId` を項目 `afterItemId` のすぐ後ろへ移す。`null` なら先頭へ移す
 * （POST /api/playlists/{id}/items/{itemId}/move）。
 */
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

/**
 * removePlaylistItem は項目を 1 つ外す（DELETE /api/playlists/{id}/items/{itemId}）。動画・グループ、
 * そのお気に入りと再生位置は変わらない。
 */
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
