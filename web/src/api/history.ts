import { apiFetch, PAGE_SIZE, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// 視聴履歴の画面 API（specs/043-watch-history/contracts/screen-api.md「Client use」）。
// 型は api/openapi.yaml からの生成物を使う。

export type WatchHistoryPage = components["schemas"]["WatchHistoryPage"];
export type WatchHistoryEntry = components["schemas"]["WatchHistoryEntry"];
export type WatchHistoryFilter = components["schemas"]["WatchHistoryFilter"];
export type WatchHistoryDates = components["schemas"]["WatchHistoryDates"];

/** 一覧と日付の一覧が共通に受ける条件（contracts/screen-api.md、research.md R-8）。 */
export interface WatchHistoryConditions {
  /** 動画のいまの視聴状態。省けば all（絞り込まない）。 */
  watch?: WatchHistoryFilter;
  /** ライブラリと同じ書き方の検索語。題名だけに照合する。空なら送らない。 */
  query?: string;
  /** 日と月の境目を取る IANA の地域名。省けばブラウザの地域（browserTimeZone）。 */
  tz?: string;
}

export interface ListWatchHistoryParams extends WatchHistoryConditions {
  /** 前回の応答の `nextCursor`。中身は解釈しない。 */
  cursor?: string;
  /** 1 ページの件数。省けば一覧と同じ 60 件。 */
  limit?: number;
  /** `YYYY-MM-DD` か `YYYY-MM`。その日か月の終わりより前の件から始める。tz と一緒に送る。 */
  date?: string;
  signal?: AbortSignal;
}

export interface ListWatchHistoryDatesParams extends WatchHistoryConditions {
  signal?: AbortSignal;
}

/** browserTimeZone はブラウザの IANA の地域名を返す（research.md R-11）。 */
export function browserTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

function setConditions(
  params: URLSearchParams,
  { watch, query }: WatchHistoryConditions,
) {
  if (watch !== undefined) params.set("watch", watch);
  if (query !== undefined && query !== "") params.set("query", query);
}

/**
 * listWatchHistory は条件を満たす視聴履歴を新しい順に 1 ページ取得する（GET /api/watch-history）。
 * 同じ表示のどのページにも同じ watch・query・date・tz を渡す。
 */
export function listWatchHistory({
  cursor,
  limit = PAGE_SIZE,
  watch,
  query,
  date,
  tz,
  signal,
}: ListWatchHistoryParams = {}): Promise<WatchHistoryPage> {
  const params = new URLSearchParams();
  if (cursor !== undefined) params.set("cursor", cursor);
  params.set("limit", String(limit));
  setConditions(params, { watch, query });
  if (date !== undefined) {
    params.set("date", date);
    params.set("tz", tz ?? browserTimeZone());
  }
  return request<WatchHistoryPage>(`/api/watch-history?${params.toString()}`, { signal });
}

/**
 * listWatchHistoryDates は条件を満たす件のある日（`YYYY-MM-DD`）を新しい順に取得する
 * （GET /api/watch-history/dates）。日と月の分け方は画面が決める。
 */
export function listWatchHistoryDates({
  watch,
  query,
  tz,
  signal,
}: ListWatchHistoryDatesParams = {}): Promise<WatchHistoryDates> {
  const params = new URLSearchParams();
  params.set("tz", tz ?? browserTimeZone());
  setConditions(params, { watch, query });
  return request<WatchHistoryDates>(`/api/watch-history/dates?${params.toString()}`, {
    signal,
  });
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
