import type { TagSort } from "../api/tags";

/** Match API TagSort values; natural name order has no direction. */
export type TagListSort = TagSort;

export type TagSortKind = "name" | "count" | "created";

export type TagSortDirection = "asc" | "desc";

export const defaultTagListSort: TagListSort = "name";

const sorts: Record<TagListSort, true> = {
  name: true,
  countDesc: true,
  countAsc: true,
  createdDesc: true,
  createdAsc: true,
};

export function isTagListSort(value: unknown): value is TagListSort {
  return typeof value === "string" && Object.hasOwn(sorts, value);
}

export const tagSortKinds: readonly { kind: TagSortKind; initial: TagListSort }[] = [
  { kind: "name", initial: "name" },
  { kind: "count", initial: "countDesc" },
  { kind: "created", initial: "createdDesc" },
];

export function tagSortKind(sort: TagListSort): TagSortKind {
  switch (sort) {
    case "name":
      return "name";
    case "countDesc":
    case "countAsc":
      return "count";
    case "createdDesc":
    case "createdAsc":
      return "created";
  }
}

export function tagSortDirection(sort: TagListSort): TagSortDirection | undefined {
  switch (sort) {
    case "name":
      return undefined;
    case "countDesc":
    case "createdDesc":
      return "desc";
    case "countAsc":
    case "createdAsc":
      return "asc";
  }
}

export function withTagSortDirection(
  sort: TagListSort,
  direction: TagSortDirection,
): TagListSort {
  switch (tagSortKind(sort)) {
    case "name":
      return "name";
    case "count":
      return direction === "desc" ? "countDesc" : "countAsc";
    case "created":
      return direction === "desc" ? "createdDesc" : "createdAsc";
  }
}
