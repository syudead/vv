import { apiFetch, PAGE_SIZE, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// 視聴履歴の画面 API（specs/043-watch-history/contracts/screen-api.md「Client use」）。
// 型は api/openapi.yaml からの生成物を使う。

export type WatchHistoryPage = components["schemas"]["WatchHistoryPage"];
export type WatchHistoryEntry = components["schemas"]["WatchHistoryEntry"];

export interface ListWatchHistoryParams {
  /** 前回の応答の `nextCursor`。中身は解釈しない。 */
  cursor?: string;
  /** 1 ページの件数。省けば一覧と同じ 60 件。 */
  limit?: number;
  signal?: AbortSignal;
}

/** listWatchHistory は視聴履歴を新しい順に 1 ページ取得する（GET /api/watch-history）。 */
export function listWatchHistory({
  cursor,
  limit = PAGE_SIZE,
  signal,
}: ListWatchHistoryParams = {}): Promise<WatchHistoryPage> {
  const query = new URLSearchParams();
  if (cursor !== undefined) query.set("cursor", cursor);
  query.set("limit", String(limit));
  return request<WatchHistoryPage>(`/api/watch-history?${query.toString()}`, { signal });
}

/**
 * deleteWatchHistoryEntry は視聴履歴の 1 件を消す（DELETE /api/watch-history/{id}）。
 * 別のところで消えていた件は 404 の RequestFailed になる（research.md R-6）。
 */
export async function deleteWatchHistoryEntry(
  id: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(`/api/watch-history/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
}

/** clearWatchHistory は視聴履歴をすべて消す（DELETE /api/watch-history）。確認は画面が持つ。 */
export async function clearWatchHistory(signal?: AbortSignal): Promise<void> {
  const response = await apiFetch("/api/watch-history", { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}
