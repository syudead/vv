import {
  isVideoSort,
  MAX_QUERY_LENGTH,
  videoSorts,
  type VideoSort,
  type WatchFilter,
} from "../api/client";
import { t, type UiText } from "../i18n";

export interface ListCriteria {
  /** Trimmed query, capped at 100 code points; empty disables search. */
  query: string;
  watch: WatchFilter;
  playable: boolean;
  /** Owner-only; serialized as fav=1. */
  favorite: boolean;
  sort: VideoSort;
  /** Defined only for random sort. */
  seed?: number;
}

export type ListCriteriaChange = ListCriteria | ((current: ListCriteria) => ListCriteria);

export const MAX_SEED = 2147483647;

export { isVideoSort, videoSorts };

export const DEFAULT_SORT: VideoSort = "addedDesc";

const watchValues: readonly WatchFilter[] = ["all", "unwatched", "inProgress", "watched"];

export type SortKind =
  | "added"
  | "modified"
  | "created"
  | "title"
  | "duration"
  | "size"
  | "played"
  | "favorited"
  | "random";

export type SortDirection = "asc" | "desc";

export interface SortKindInfo {
  kind: SortKind;
  initial: VideoSort;
  asc?: VideoSort;
  desc?: VideoSort;
}

/** Keep menu order stable and read labels from the catalog during rendering. */
export const sortKinds: readonly SortKindInfo[] = [
  { kind: "added", initial: "addedDesc", asc: "addedAsc", desc: "addedDesc" },
  { kind: "modified", initial: "modifiedDesc", asc: "modifiedAsc", desc: "modifiedDesc" },
  { kind: "created", initial: "createdDesc", asc: "createdAsc", desc: "createdDesc" },
  { kind: "title", initial: "titleAsc", asc: "titleAsc", desc: "titleDesc" },
  { kind: "duration", initial: "durationDesc", asc: "durationAsc", desc: "durationDesc" },
  { kind: "size", initial: "sizeDesc", asc: "sizeAsc", desc: "sizeDesc" },
  { kind: "played", initial: "playedDesc", asc: "playedAsc", desc: "playedDesc" },
  {
    kind: "favorited",
    initial: "favoritedDesc",
    asc: "favoritedAsc",
    desc: "favoritedDesc",
  },
  { kind: "random", initial: "random" },
];

/**
 * Accept only API sorts represented in the menu; others would display the wrong kind and break
 * direction controls.
 */
export function isListSort(value: unknown): value is VideoSort {
  return (
    isVideoSort(value) &&
    sortKinds.some(
      (info) => info.initial === value || info.asc === value || info.desc === value,
    )
  );
}

export function sortKindLabel(kind: SortKind): UiText {
  return t.list.sort.kinds[kind];
}

function directionWording(kind: SortKind, direction: SortDirection): UiText | undefined {
  const wording: Partial<Record<SortKind, Record<SortDirection, UiText>>> =
    t.list.sort.wording;
  return wording[kind]?.[direction];
}

export function sortKindOf(sort: VideoSort): SortKindInfo {
  const found = sortKinds.find((info) => info.asc === sort || info.desc === sort);
  // Random has neither asc nor desc, so unmatched direction values belong to it.
  return found ?? (sortKinds[sortKinds.length - 1] as SortKindInfo);
}

export function sortDirection(sort: VideoSort): SortDirection | undefined {
  if (sort === "random") return undefined;
  return sort.endsWith("Asc") ? "asc" : "desc";
}

export function withDirection(sort: VideoSort, direction: SortDirection): VideoSort {
  const info = sortKindOf(sort);
  return (direction === "asc" ? info.asc : info.desc) ?? sort;
}

export function directionLabel(sort: VideoSort, direction: SortDirection): UiText {
  const wording = directionWording(sortKindOf(sort).kind, direction);
  const name = direction === "asc" ? t.list.sort.asc : t.list.sort.desc;
  return wording === undefined ? name : t.list.sort.withWording(name, wording);
}

export function directionToggleLabel(sort: VideoSort): UiText {
  const current = sortDirection(sort) ?? "desc";
  const label = directionLabel(sort, current);
  return current === "asc" ? t.list.sort.toDesc(label) : t.list.sort.toAsc(label);
}

/** maxLength counts code points and defaults to the video-list limit. */
export function normalizeQuery(value: string, maxLength = MAX_QUERY_LENGTH): string {
  // Match Go's rune count without splitting surrogate pairs.
  return Array.from(value.trim()).slice(0, maxLength).join("").trim();
}

export function parseSeed(value: string | null): number | undefined {
  if (value === null || !/^\d{1,10}$/.test(value)) return undefined;
  const seed = Number(value);
  return seed >= 1 && seed <= MAX_SEED ? seed : undefined;
}

/** Avoid the previous seed so reshuffling changes the order. */
export function newSeed(previous?: number, random: () => number = Math.random): number {
  for (;;) {
    const seed = 1 + Math.floor(random() * MAX_SEED);
    const bounded = Math.min(Math.max(seed, 1), MAX_SEED);
    if (bounded !== previous) return bounded;
  }
}

export interface ParsedCriteria {
  criteria: ListCriteria;
  /** The screen must generate a missing or invalid random seed and replace the URL. */
  needsSeed: boolean;
  hasExplicitSort: boolean;
}

/**
 * Invalid or omitted sort falls back to stored preferredSort, including API values absent from the
 * menu.
 */
export function parseListCriteria(
  params: URLSearchParams,
  preferredSort: VideoSort = DEFAULT_SORT,
): ParsedCriteria {
  const rawSort = params.get("sort");
  const hasExplicitSort = isListSort(rawSort);
  const sort: VideoSort = hasExplicitSort ? rawSort : preferredSort;
  const rawWatch = params.get("watch");
  const watch =
    rawWatch !== null && (watchValues as readonly string[]).includes(rawWatch)
      ? (rawWatch as WatchFilter)
      : "all";
  const seed = sort === "random" ? parseSeed(params.get("seed")) : undefined;
  return {
    criteria: {
      query: normalizeQuery(params.get("q") ?? ""),
      watch,
      playable: params.get("playable") === "1",
      favorite: params.get("fav") === "1",
      sort,
      ...(seed === undefined ? {} : { seed }),
    },
    needsSeed: sort === "random" && seed === undefined,
    hasExplicitSort,
  };
}

/**
 * Always serialize sort so browser Back restores order independently of changed preferences;
 * serialize seed only for random. Omit filter defaults and keep stable parameter order.
 */
export function serializeListCriteria(criteria: ListCriteria): URLSearchParams {
  const params = new URLSearchParams();
  const query = normalizeQuery(criteria.query);
  if (query !== "") params.set("q", query);
  if (criteria.watch !== "all") params.set("watch", criteria.watch);
  if (criteria.playable) params.set("playable", "1");
  if (criteria.favorite) params.set("fav", "1");
  params.set("sort", criteria.sort);
  if (criteria.sort === "random" && criteria.seed !== undefined) {
    params.set("seed", String(criteria.seed));
  }
  return params;
}

/**
 * Playback and favorite dates are owner data; guests and server guest requests cannot use these
 * sorts.
 */
export const ownerOnlySortKinds: readonly SortKind[] = ["played", "favorited"];

export function ownerOnlySort(sort: VideoSort): boolean {
  return (
    sort === "playedAsc" ||
    sort === "playedDesc" ||
    sort === "favoritedAsc" ||
    sort === "favoritedDesc"
  );
}

/** Normalize owner-only conditions for guests; preserve object identity when unchanged. */
export function guestListCriteria(criteria: ListCriteria): ListCriteria {
  if (criteria.watch === "all" && !criteria.favorite && !ownerOnlySort(criteria.sort)) {
    return criteria;
  }
  return {
    ...criteria,
    watch: "all",
    favorite: false,
    sort: ownerOnlySort(criteria.sort) ? DEFAULT_SORT : criteria.sort,
  };
}

export function hasConditions(criteria: ListCriteria): boolean {
  return (
    criteria.query !== "" ||
    criteria.watch !== "all" ||
    criteria.playable ||
    criteria.favorite
  );
}

/** Clear filters while retaining sort and seed. */
export function clearConditions(criteria: ListCriteria): ListCriteria {
  return { ...criteria, query: "", watch: "all", playable: false, favorite: false };
}

export function criteriaKey(criteria: ListCriteria): string {
  return serializeListCriteria(criteria).toString();
}

export type HistoryMode = "push" | "replace";

/** A focus-to-blur/Escape session pushes history only on its first commit; later commits replace. */
export class SearchSession {
  private pushed = false;

  start(): void {
    this.pushed = false;
  }

  commit(): HistoryMode {
    if (this.pushed) return "replace";
    this.pushed = true;
    return "push";
  }

  end(): void {
    this.pushed = false;
  }
}
