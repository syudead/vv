import type { WatchHistoryFilter } from "../api/history";
import { normalizeQuery } from "../videoList/listCriteria";

/** Omit defaults from URLs and normalize invalid values to defaults. */

export interface HistoryCriteria {
  watch: WatchHistoryFilter;
  /** Trimmed title query, capped at 100 Unicode code points; empty disables search. */
  query: string;
  /** YYYY-MM-DD or YYYY-MM; absent starts with the latest records. */
  date?: string;
}

export const DEFAULT_HISTORY_CRITERIA: HistoryCriteria = { watch: "all", query: "" };

const watchValues: readonly WatchHistoryFilter[] = ["all", "inProgress", "watched"];

function isWatchFilter(value: string | null): value is WatchHistoryFilter {
  return value !== null && (watchValues as readonly string[]).includes(value);
}

/** Accept only valid calendar days or months. */
export function parseHistoryDate(value: string | null): string | undefined {
  if (value === null) return undefined;
  const match = /^(\d{4})-(\d{2})(?:-(\d{2}))?$/.exec(value);
  if (match === null) return undefined;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (year < 1 || month < 1 || month > 12) return undefined;
  if (match[3] === undefined) return value;
  const day = Number(match[3]);
  // Day zero of the next month gives this month's length.
  const days = new Date(Date.UTC(year, month, 0)).getUTCDate();
  return day >= 1 && day <= days ? value : undefined;
}

export function parseHistoryCriteria(params: URLSearchParams): HistoryCriteria {
  const watch = params.get("watch");
  const date = parseHistoryDate(params.get("date"));
  return {
    watch: isWatchFilter(watch) ? watch : "all",
    query: normalizeQuery(params.get("q") ?? ""),
    ...(date === undefined ? {} : { date }),
  };
}

/** Omit defaults and serialize in stable watch, q, date order. */
export function serializeHistoryCriteria(criteria: HistoryCriteria): URLSearchParams {
  const params = new URLSearchParams();
  if (criteria.watch !== "all") params.set("watch", criteria.watch);
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  const date = parseHistoryDate(criteria.date ?? null);
  if (date !== undefined) params.set("date", date);
  return params;
}

export function hasHistoryConditions(criteria: HistoryCriteria): boolean {
  return criteria.watch !== "all" || criteria.query !== "" || criteria.date !== undefined;
}

export function historyCriteriaKey(criteria: HistoryCriteria): string {
  return serializeHistoryCriteria(criteria).toString();
}
