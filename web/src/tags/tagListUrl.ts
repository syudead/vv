import { isTagListSort, type TagListSort } from "../api/tags";
import { useUrlCriteria } from "../recordList/useUrlCriteria";
import { type HistoryMode, normalizeQuery } from "../videoList/listCriteria";

export type TagListTab = "tags" | "rejected" | "kinds";

/** Positive ID string or none; null means unfiltered. */
export type TagKindFilter = string | null;

function parseTagListTab(value: string | null): TagListTab {
  return value === "rejected" || value === "kinds" ? value : "tags";
}

function parseTagKindFilter(value: string | null): TagKindFilter {
  if (value === null) return null;
  return value === "none" || /^[1-9]\d*$/.test(value) ? value : null;
}

export interface TagListCriteria {
  query: string;
  tentativeOnly: boolean;
  unusedOnly: boolean;
  kind: TagKindFilter;
  sort: TagListSort;
  tab: TagListTab;
}

/** Invalid values fall back silently; missing/invalid sort uses the device preference. */
export function parseTagListCriteria(
  params: URLSearchParams,
  preferredSort: TagListSort,
): { criteria: TagListCriteria; hasExplicitSort: boolean } {
  const rawSort = params.get("sort");
  const hasExplicitSort = isTagListSort(rawSort);
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      tentativeOnly: params.get("tentative") === "1",
      unusedOnly: params.get("unused") === "1",
      kind: parseTagKindFilter(params.get("kind")),
      sort: hasExplicitSort ? rawSort : preferredSort,
      tab: parseTagListTab(params.get("tab")),
    },
    hasExplicitSort,
  };
}

/** Always serialize sort so Back restores prior order instead of a changed preference; omit false filters and the default tab. */
export function serializeTagListCriteria(criteria: TagListCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  if (criteria.tentativeOnly) params.set("tentative", "1");
  if (criteria.unusedOnly) params.set("unused", "1");
  if (criteria.kind !== null) params.set("kind", criteria.kind);
  params.set("sort", criteria.sort);
  if (criteria.tab !== "tags") params.set("tab", criteria.tab);
  return params;
}

/** Push discrete actions; replace continued search input. */
export function useTagListCriteria(preferredSort: TagListSort): {
  criteria: TagListCriteria;
  apply: (change: Partial<TagListCriteria>, mode: HistoryMode) => void;
} {
  return useUrlCriteria(
    (params) => parseTagListCriteria(params, preferredSort),
    serializeTagListCriteria,
  );
}
