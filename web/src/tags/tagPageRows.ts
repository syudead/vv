import type { Tag } from "../api/tags";
import { foldForMatch } from "../lib/foldForMatch";
import { compareNaturalSortKeys, naturalSortKey } from "../lib/naturalSortKey";
import type { TagListSort } from "./tagListOrder";

/**
 * TagRowsQuery は、読み込んだ行（`rows`）をサーバーから受けたときの条件である
 * （specs/036-tag-admin-scale/data-model.md §4「条件」）。`query` は検索語に
 * `foldForMatch` を掛けて前後の空白を落とした照合形で、空なら絞らない。
 */
export interface TagRowsQuery {
  query: string;
  tentativeOnly: boolean;
  unusedOnly: boolean;
  sort: TagListSort;
}

/**
 * matchesTagQuery は、タグが条件の検索・絞り込みに合うかである。サーバーの
 * `GET /api/tags` と同じ規則（検索語の照合形が元の名前かシノニムの照合形に部分一致、
 * 「Tentative only」は仮のタグ、「Unused only」は本数 0）で、操作のあとの 1 行を
 * 往復せずに判定するのに使う（research.md R-3・R-12）。
 */
export function matchesTagQuery(tag: Tag, query: TagRowsQuery): boolean {
  if (query.tentativeOnly && !tag.tentative) return false;
  if (query.unusedOnly && tag.videoCount !== 0) return false;
  if (query.query === "") return true;
  return (
    foldForMatch(tag.name).includes(query.query) ||
    tag.synonyms.some((synonym) => foldForMatch(synonym).includes(query.query))
  );
}

/**
 * createdSeconds は作った日時の Unix 秒である。サーバーは `tags.created_at`（秒）で
 * 並べるので、同じ秒のタグは名前の順になる（research.md R-8）。読めなければ 0。
 */
function createdSeconds(tag: Tag): number {
  const time = Date.parse(tag.createdAt);
  return Number.isNaN(time) ? 0 : Math.floor(time / 1000);
}

/**
 * compareTagsForSort はサーバーの並び（data-model.md §2 の `order by`）と同じ前後を
 * 返す。本数・作った日の並びでは先にその値で比べ、同じなら名前の自然順の鍵
 * （`naturalSortKey`）、それも同じなら `id` で比べる。`keys` は鍵の控えで、同じ
 * タグの鍵を何度も作らない。
 */
export function compareTagsForSort(
  a: Tag,
  b: Tag,
  sort: TagListSort,
  keys: (tag: Tag) => string = (tag) => naturalSortKey(tag.name),
): number {
  let byValue = 0;
  switch (sort) {
    case "countDesc":
      byValue = b.videoCount - a.videoCount;
      break;
    case "countAsc":
      byValue = a.videoCount - b.videoCount;
      break;
    case "createdDesc":
      byValue = createdSeconds(b) - createdSeconds(a);
      break;
    case "createdAsc":
      byValue = createdSeconds(a) - createdSeconds(b);
      break;
    case "name":
      break;
  }
  if (byValue !== 0) return Math.sign(byValue);
  const byKey = compareNaturalSortKeys(keys(a), keys(b));
  if (byKey !== 0) return byKey;
  return Math.sign(a.id - b.id);
}

/**
 * insertionIndex は、並んだ `rows` のどこに `tag` が入るかである（`tag` 自身は
 * `rows` に無いものとする）。行は並び順で並んでいるので二分探索で探し、比べる
 * 名前の鍵は `tag` の分を 1 回だけ作る。
 */
export function insertionIndex(
  rows: readonly Tag[],
  tag: Tag,
  sort: TagListSort,
): number {
  const ownKey = naturalSortKey(tag.name);
  const keys = (item: Tag) => (item === tag ? ownKey : naturalSortKey(item.name));
  let low = 0;
  let high = rows.length;
  while (low < high) {
    const middle = (low + high) >>> 1;
    if (compareTagsForSort(rows[middle]!, tag, sort, keys) < 0) low = middle + 1;
    else high = middle;
  }
  return low;
}

/**
 * placeTag は、`tag` を `rows` の並び順の位置へ置いた新しい配列を返す。同じ id の
 * 行があれば先に取り除く。`boundary` は続きのカーソルを作った行（読み込んだ最後の
 * ページの最後の行の、受けたときの状態）で、続きが無ければ undefined。`tag` が
 * それより後ろに並ぶなら読み込んだ範囲の外で、置かない。その行は続きのページが
 * 返すので、置くと続きと二重になる。範囲の中なら置く。続きは境より後ろだけを返す
 * ので、範囲の中へ動いた行は画面が置かなければ抜ける（data-model.md §4「操作の
 * あとの反映」、research.md R-12）。
 */
export function placeTag(
  rows: readonly Tag[],
  tag: Tag,
  sort: TagListSort,
  boundary: Tag | undefined,
): Tag[] {
  const rest = rows.filter((item) => item.id !== tag.id);
  if (boundary !== undefined && compareTagsForSort(tag, boundary, sort) > 0) return rest;
  const index = insertionIndex(rest, tag, sort);
  return [...rest.slice(0, index), tag, ...rest.slice(index)];
}

/**
 * appendUniqueTags は続きのページから、既に読み込んだ行と同じ id の行を捨てて末尾に
 * 足す（`web/src/api/videosData.ts` の `appendUnique` と同じ。research.md R-11）。
 * 足すものが無ければ受け取った配列をそのまま返す。
 */
export function appendUniqueTags(rows: readonly Tag[], next: readonly Tag[]): Tag[] {
  const seen = new Set(rows.map((tag) => tag.id));
  const added: Tag[] = [];
  for (const tag of next) {
    if (seen.has(tag.id)) continue;
    seen.add(tag.id);
    added.push(tag);
  }
  return added.length === 0 ? (rows as Tag[]) : [...rows, ...added];
}

/**
 * TagPageRows は読み込んだ行と件数である（data-model.md §4「ページ」）。`total` は
 * 条件に合うタグの数、`totalAll` は全部のタグの数で、どちらも読み込んでいない
 * タグを含む。`nextCursor` は続きがあるときだけ入る。
 */
export interface TagPageRows {
  rows: Tag[];
  total: number;
  totalAll: number;
  nextCursor: string | undefined;
  /**
   * boundary は `nextCursor` を作った行（そのページの最後の行の、受けたときの
   * 状態）である。続きが無ければ undefined。操作のあとに置く行が読み込んだ範囲の
   * 中かを、これと比べて決める（`placeTag`）。
   */
  boundary: Tag | undefined;
  query: TagRowsQuery;
}

/**
 * replaceTag は、タグ 1 件が書き換わった（確定・改名・統合先）ことを読み込んだ行へ
 * 反映する。`before` は書き換わる前の状態（行か統合の窓の候補。分からなければ
 * undefined）で、前後それぞれを今の条件に照らして `total` を増減し、合えば並び順の
 * 位置へ置き直し、合わなければ取り除く（data-model.md §4「操作のあとの反映」）。
 */
export function replaceTag(
  page: TagPageRows,
  before: Tag | undefined,
  after: Tag,
): TagPageRows {
  // 読み込んだ行にあったタグはサーバーが条件に合うとしたもので、照らし直さない。
  const was =
    before !== undefined &&
    (page.rows.some((tag) => tag.id === before.id) ||
      matchesTagQuery(before, page.query));
  const now = matchesTagQuery(after, page.query);
  const rows = now
    ? placeTag(page.rows, after, page.query.sort, page.boundary)
    : page.rows.filter((tag) => tag.id !== after.id);
  return { ...page, rows, total: page.total + Number(now) - Number(was) };
}

/**
 * removeTags は、もう無いタグ（却下・削除・統合元）を読み込んだ行から取り除き、
 * `total`・`totalAll` を減らす。`removed` は消えたタグの消える前の状態で、今の条件に
 * 合っていたものだけを `total` から引く。
 */
export function removeTags(page: TagPageRows, removed: readonly Tag[]): TagPageRows {
  if (removed.length === 0) return page;
  const ids = new Set(removed.map((tag) => tag.id));
  const rows = page.rows.filter((tag) => !ids.has(tag.id));
  // 読み込んだ行にあったタグはサーバーが条件に合うとしたもので、照らし直さない。
  const loaded = page.rows.length - rows.length;
  const loadedIds = new Set(page.rows.map((tag) => tag.id));
  const others = removed.filter(
    (tag) => !loadedIds.has(tag.id) && matchesTagQuery(tag, page.query),
  ).length;
  return {
    ...page,
    rows,
    total: Math.max(0, page.total - loaded - others),
    totalAll: Math.max(0, page.totalAll - ids.size),
  };
}

/**
 * confirmTags は、まとめて確定したタグの行を `tentative: false` に差し替える。確定は
 * 並び順の値を変えないので位置はそのままで、「Tentative only」が効いていれば
 * 取り除いて `total` を減らす。行の数によらず 1 回の走査で済ませる。
 */
export function confirmTags(page: TagPageRows, ids: ReadonlySet<number>): TagPageRows {
  if (ids.size === 0) return page;
  let removed = 0;
  const rows: Tag[] = [];
  for (const tag of page.rows) {
    if (!ids.has(tag.id) || !tag.tentative) {
      rows.push(tag);
      continue;
    }
    const confirmed = { ...tag, tentative: false };
    if (matchesTagQuery(confirmed, page.query)) rows.push(confirmed);
    else removed += 1;
  }
  return { ...page, rows, total: Math.max(0, page.total - removed) };
}

/**
 * addTag は作ったタグを反映する。今の条件に合えば並び順の位置へ置き `total` を
 * 増やし、合わなければ `totalAll` だけを増やす。
 */
export function addTag(page: TagPageRows, created: Tag): TagPageRows {
  const next = replaceTag(page, undefined, created);
  return { ...next, totalAll: page.totalAll + 1 };
}
