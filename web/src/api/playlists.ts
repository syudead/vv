import { request } from "./client";
import type { components } from "./gen/openapi";

// プレイリストの画面 API（specs/044-playlists/contracts/screen-api.md「Client use」）。
// 型は api/openapi.yaml からの生成物を使う。

export type PlaylistSummary = components["schemas"]["PlaylistSummary"];
export type PlaylistPlayback = components["schemas"]["PlaylistPlayback"];
export type PlaylistStep = components["schemas"]["PlaylistStep"];

/**
 * getPlaylistPlayback は、動画ページがプレイリスト `playlistId` の項目 `itemId` の動画 `videoId` を
 * 再生しているときの位置と次の動画を、その時点の並びから取得する
 * （GET /api/playlists/{id}/playback、research.md R-6）。プレイリストか項目が無くなっていれば
 * 404 の RequestFailed になり、画面はプレイリストの文脈を外す（research.md R-7）。
 */
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
