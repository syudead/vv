import { compareTagRefs } from "../api/tagOrder";
import type { Tag } from "../api/tags";

/**
 * TagListSort はタグ管理画面の一覧の並び順である
 * （specs/036-tag-admin-scale/data-model.md §4、research.md R-7）。
 * 「名前」に向きは無い。
 */
export type TagListSort =
  "name" | "countDesc" | "countAsc" | "createdDesc" | "createdAsc";

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

/**
 * sortTags はタグを並び順に並べた新しい配列を返す。値が同じタグどうしは
 * 名前の自然順（`compareTagRefs`）で並べる（research.md R-7・R-8）。
 * 「作った日」は `createdAt` を時刻として比べ、比べる値は並べる前に1回だけ
 * 読む。
 */
export function sortTags(tags: readonly Tag[], sort: TagListSort): Tag[] {
  const sorted = [...tags];
  switch (sort) {
    case "name":
      return sorted.sort(compareTagRefs);
    case "countDesc":
    case "countAsc": {
      const sign = sort === "countDesc" ? -1 : 1;
      return sorted.sort(
        (a, b) => sign * (a.videoCount - b.videoCount) || compareTagRefs(a, b),
      );
    }
    case "createdDesc":
    case "createdAsc": {
      const sign = sort === "createdDesc" ? -1 : 1;
      const times = new Map(sorted.map((tag) => [tag, createdTime(tag)]));
      return sorted.sort(
        (a, b) => sign * (times.get(a)! - times.get(b)!) || compareTagRefs(a, b),
      );
    }
  }
}

/** createdTime は作った日時のミリ秒である。読めなければ 0（最も古い）とする。 */
function createdTime(tag: Tag): number {
  const time = Date.parse(tag.createdAt);
  return Number.isNaN(time) ? 0 : time;
}
