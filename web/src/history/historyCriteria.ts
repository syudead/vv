import type { WatchHistoryFilter } from "../api/history";
import { normalizeQuery } from "../videoList/listCriteria";

/**
 * 視聴履歴の画面の条件と URL の相互変換（specs/043-watch-history/contracts/screen-api.md
 * 「Client use」、research.md R-12）。ライブラリの listCriteria.ts と同じく、既定値は書かず、
 * 読めない値は既定値として扱う。React に依存しない純粋な関数だけを置く。
 */

/** HistoryCriteria は履歴の一覧を決める条件である。 */
export interface HistoryCriteria {
  /** 動画のいまの視聴状態。all は絞り込まない。 */
  watch: WatchHistoryFilter;
  /** 題名の検索語。前後の空白を落とし、100 符号位置で切ったもの。空なら検索しない。 */
  query: string;
  /** 移った日（`YYYY-MM-DD`）か月（`YYYY-MM`）。無ければ最新から。 */
  date?: string;
}

export const DEFAULT_HISTORY_CRITERIA: HistoryCriteria = { watch: "all", query: "" };

const watchValues: readonly WatchHistoryFilter[] = ["all", "inProgress", "watched"];

function isWatchFilter(value: string | null): value is WatchHistoryFilter {
  return value !== null && (watchValues as readonly string[]).includes(value);
}

/**
 * parseHistoryDate は `date` を読む。暦にある日（`YYYY-MM-DD`）か月（`YYYY-MM`）でなければ
 * undefined。
 */
export function parseHistoryDate(value: string | null): string | undefined {
  if (value === null) return undefined;
  const match = /^(\d{4})-(\d{2})(?:-(\d{2}))?$/.exec(value);
  if (match === null) return undefined;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (year < 1 || month < 1 || month > 12) return undefined;
  if (match[3] === undefined) return value;
  const day = Number(match[3]);
  // 月の日数は翌月の 0 日で求める。
  const days = new Date(Date.UTC(year, month, 0)).getUTCDate();
  return day >= 1 && day <= days ? value : undefined;
}

/** parseHistoryCriteria は URL のクエリを条件にする。読めない値は既定にする。 */
export function parseHistoryCriteria(params: URLSearchParams): HistoryCriteria {
  const watch = params.get("watch");
  const date = parseHistoryDate(params.get("date"));
  return {
    watch: isWatchFilter(watch) ? watch : "all",
    query: normalizeQuery(params.get("q") ?? ""),
    ...(date === undefined ? {} : { date }),
  };
}

/**
 * serializeHistoryCriteria は条件を URL のクエリにする。watch=all、空の検索語、無い日は
 * 書かない。順は watch・q・date で固定し、同じ条件は同じ文字列になる。
 */
export function serializeHistoryCriteria(criteria: HistoryCriteria): URLSearchParams {
  const params = new URLSearchParams();
  if (criteria.watch !== "all") params.set("watch", criteria.watch);
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  const date = parseHistoryDate(criteria.date ?? null);
  if (date !== undefined) params.set("date", date);
  return params;
}

/** hasHistoryConditions は「Clear filters」で外せる条件（状態・検索語・日）があるか。 */
export function hasHistoryConditions(criteria: HistoryCriteria): boolean {
  return criteria.watch !== "all" || criteria.query !== "" || criteria.date !== undefined;
}

/** historyCriteriaKey は条件を比較・依存配列に使う文字列にする。 */
export function historyCriteriaKey(criteria: HistoryCriteria): string {
  return serializeHistoryCriteria(criteria).toString();
}
