import { apiFetch, PAGE_SIZE, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

export type WatchHistoryPage = components["schemas"]["WatchHistoryPage"];
export type WatchHistoryEntry = components["schemas"]["WatchHistoryEntry"];
export type WatchHistoryFilter = components["schemas"]["WatchHistoryFilter"];
export type WatchHistoryDates = components["schemas"]["WatchHistoryDates"];

export interface WatchHistoryConditions {
  watch?: WatchHistoryFilter;
  /** Library query syntax, matching titles only. */
  query?: string;
  /** IANA timezone for date/month boundaries; defaults to the browser timezone. */
  tz?: string;
}

export interface ListWatchHistoryParams extends WatchHistoryConditions {
  /** Opaque nextCursor from the previous response. */
  cursor?: string;
  limit?: number;
  /** Start before this YYYY-MM-DD or YYYY-MM period ends; send with tz. */
  date?: string;
  signal?: AbortSignal;
}

export interface ListWatchHistoryDatesParams extends WatchHistoryConditions {
  signal?: AbortSignal;
}

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

/** Keep watch, query, date, and tz identical across pages of the same view. */
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

export async function clearWatchHistory(signal?: AbortSignal): Promise<void> {
  const response = await apiFetch("/api/watch-history", { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}
