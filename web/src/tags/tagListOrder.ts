import type { TagSort } from "../api/tags";

/**
 * TagListSort はタグ管理画面の一覧の並び順である
 * （specs/036-tag-admin-scale/data-model.md §4、research.md R-7）。
 * `GET /api/tags` の `sort`（生成物の `TagSort`）と同じ値を持つ。「名前」に向きは無い。
 */
export type TagListSort = TagSort;

/** TagSortKind は並び順の種類（メニューの項目）である。 */
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

/** tagSortKinds はメニューに並べる種類と、選んだときの既定の並び順である。 */
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

/** tagSortDirection は並び順の向きである。「名前」は向きを持たない。 */
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

/** withTagSortDirection は種類を保ったまま向きを変える。「名前」はそのまま返す。 */
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
