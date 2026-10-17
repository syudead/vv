import type { TagKindRef } from "../api/tagKinds";
import type { Tag } from "../api/tags";
import { foldForMatch } from "../lib/foldForMatch";
import { compareNaturalSortKeys, naturalSortKey } from "../lib/naturalSortKey";
import type { TagListSort } from "./tagListOrder";
import type { TagKindFilter } from "./tagListUrl";

/**
 * ListRow は、サーバーのページで読んで並べる管理画面の 1 行（タグ、人物、団体）の、並び順と
 * 位置の計算が読む部分である。タグの画面と人物・団体の画面（`web/src/credits/`）が、行の
 * 置き直しと件数の数え直しをこのモジュールで共有する。
 */
export interface ListRow {
  id: number;
  name: string;
  videoCount: number;
  createdAt: string;
}

/** ListRowsQuery は、読み込んだ行を受けたときの条件の、どの画面にもある部分である。 */
export interface ListRowsQuery {
  /** 検索語の照合形（`foldForMatch` を掛けて前後の空白を落としたもの）。空なら絞らない。 */
  query: string;
  sort: TagListSort;
}

/** RowMatcher は、1 行が条件に合うかをサーバーと同じ規則で決める。 */
export type RowMatcher<R, Q> = (row: R, query: Q) => boolean;

/**
 * TagRowsQuery は、読み込んだ行（`rows`）をサーバーから受けたときの条件である。`query` は検索語に
 * `foldForMatch` を掛けて前後の空白を落とした照合形で、空なら絞らない。
 */
export interface TagRowsQuery extends ListRowsQuery {
  tentativeOnly: boolean;
  unusedOnly: boolean;
  /** 種類の絞り込み。省くか null なら絞らない。 */
  kind?: TagKindFilter;
}

/**
 * matchesKind は、タグが種類の絞り込みに合うかである。`none` は種類の無いタグ、id はその
 * 種類のタグに合う。
 */
export function matchesKind(tag: Tag, kind: TagKindFilter | undefined): boolean {
  if (kind === undefined || kind === null) return true;
  if (kind === "none") return tag.kind === null;
  return tag.kind !== null && String(tag.kind.id) === kind;
}

/**
 * matchesTagQuery は、タグが条件の検索・絞り込みに合うかである。サーバーの
 * `GET /api/tags` と同じ規則（検索語の照合形が元の名前かシノニムの照合形に部分一致、
 * 「Tentative only」は仮のタグ、「Unused only」は本数 0）で、操作のあとの 1 行を
 * 往復せずに判定するのに使う。
 */
export function matchesTagQuery(tag: Tag, query: TagRowsQuery): boolean {
  if (query.tentativeOnly && !tag.tentative) return false;
  if (query.unusedOnly && tag.videoCount !== 0) return false;
  if (!matchesKind(tag, query.kind)) return false;
  if (query.query === "") return true;
  return (
    foldForMatch(tag.name).includes(query.query) ||
    tag.synonyms.some((synonym) => foldForMatch(synonym).includes(query.query))
  );
}

/**
 * createdSeconds は作った日時の Unix 秒である。サーバーは `tags.created_at`（秒）で
 * 並べるので、同じ秒のタグは名前の順になる。読めなければ 0。
 */
function createdSeconds(tag: ListRow): number {
  const time = Date.parse(tag.createdAt);
  return Number.isNaN(time) ? 0 : Math.floor(time / 1000);
}

/**
 * compareTagsForSort はサーバーの並びと同じ前後を
 * 返す。本数・作った日の並びでは先にその値で比べ、同じなら名前の自然順の鍵
 * （`naturalSortKey`）、それも同じなら `id` で比べる。`keys` は鍵の控えで、同じ
 * タグの鍵を何度も作らない。
 */
export function compareTagsForSort<R extends ListRow>(
  a: R,
  b: R,
  sort: TagListSort,
  keys: (tag: R) => string = (tag) => naturalSortKey(tag.name),
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
export function insertionIndex<R extends ListRow>(
  rows: readonly R[],
  tag: R,
  sort: TagListSort,
): number {
  const ownKey = naturalSortKey(tag.name);
  const keys = (item: R) => (item === tag ? ownKey : naturalSortKey(item.name));
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
 * ので、範囲の中へ動いた行は画面が置かなければ抜ける。
 */
export function placeTag<R extends ListRow>(
  rows: readonly R[],
  tag: R,
  sort: TagListSort,
  boundary: R | undefined,
): R[] {
  const rest = rows.filter((item) => item.id !== tag.id);
  if (boundary !== undefined && compareTagsForSort(tag, boundary, sort) > 0) return rest;
  const index = insertionIndex(rest, tag, sort);
  return [...rest.slice(0, index), tag, ...rest.slice(index)];
}

/**
 * appendUniqueTags は続きのページから、既に読み込んだ行と同じ id の行を捨てて末尾に
 * 足す（`web/src/api/videosData.ts` の `appendUnique` と同じ）。
 * 足すものが無ければ受け取った配列をそのまま返す。
 */
export function appendUniqueTags<R extends ListRow>(
  rows: readonly R[],
  next: readonly R[],
): R[] {
  const seen = new Set(rows.map((tag) => tag.id));
  const added: R[] = [];
  for (const tag of next) {
    if (seen.has(tag.id)) continue;
    seen.add(tag.id);
    added.push(tag);
  }
  return added.length === 0 ? (rows as R[]) : [...rows, ...added];
}

/**
 * PageRows は読み込んだ行と件数である。`total` は
 * 条件に合う行の数、`totalAll` は全部の数で、どちらも読み込んでいない行を含む。
 * `nextCursor` は続きがあるときだけ入る。
 */
export interface PageRows<R extends ListRow, Q extends ListRowsQuery> {
  rows: R[];
  total: number;
  totalAll: number;
  nextCursor: string | undefined;
  /**
   * boundary は `nextCursor` を作った行（そのページの最後の行の、受けたときの
   * 状態）である。続きが無ければ undefined。操作のあとに置く行が読み込んだ範囲の
   * 中かを、これと比べて決める（`placeTag`）。
   */
  boundary: R | undefined;
  query: Q;
}

/** TagPageRows はタグ管理画面の読み込んだ行と件数である。 */
export type TagPageRows = PageRows<Tag, TagRowsQuery>;

/**
 * replaceTag は、タグ 1 件が書き換わった（確定・改名・統合先）ことを読み込んだ行へ
 * 反映する。`before` は書き換わる前の状態（行か統合の窓の候補。分からなければ
 * undefined）で、前後それぞれを今の条件に照らして `total` を増減し、合えば並び順の
 * 位置へ置き直し、合わなければ取り除く。
 */
export function replaceTag(
  page: TagPageRows,
  before: Tag | undefined,
  after: Tag,
): TagPageRows {
  return replaceRow(page, before, after, matchesTagQuery);
}

/** replaceRow は `replaceTag` の、条件の判定（`matches`）を受け取る形である。 */
export function replaceRow<R extends ListRow, Q extends ListRowsQuery>(
  page: PageRows<R, Q>,
  before: R | undefined,
  after: R,
  matches: RowMatcher<R, Q>,
): PageRows<R, Q> {
  // 読み込んだ行にあったタグはサーバーが条件に合うとしたもので、照らし直さない。
  const was =
    before !== undefined &&
    (page.rows.some((tag) => tag.id === before.id) || matches(before, page.query));
  const now = matches(after, page.query);
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
  return removeRows(page, removed, matchesTagQuery);
}

/** removeRows は `removeTags` の、条件の判定（`matches`）を受け取る形である。 */
export function removeRows<R extends ListRow, Q extends ListRowsQuery>(
  page: PageRows<R, Q>,
  removed: readonly R[],
  matches: RowMatcher<R, Q>,
): PageRows<R, Q> {
  if (removed.length === 0) return page;
  const ids = new Set(removed.map((tag) => tag.id));
  const rows = page.rows.filter((tag) => !ids.has(tag.id));
  // 読み込んだ行にあったタグはサーバーが条件に合うとしたもので、照らし直さない。
  const loaded = page.rows.length - rows.length;
  const loadedIds = new Set(page.rows.map((tag) => tag.id));
  const others = removed.filter(
    (tag) => !loadedIds.has(tag.id) && matches(tag, page.query),
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
  return updateTags(page, ids, (tag) =>
    tag.tentative ? { ...tag, tentative: false } : tag,
  );
}

/**
 * setTagsKind は、まとめて種類を付けた（`kind` が null なら外した）タグの行の種類を
 * 書き換える。種類は並び順の値を変えないので
 * 位置はそのままで、種類の絞り込みに合わなくなった行は取り除いて `total` を減らす。
 */
export function setTagsKind(
  page: TagPageRows,
  ids: ReadonlySet<number>,
  kind: TagKindRef | null,
): TagPageRows {
  return updateTags(page, ids, (tag) => ({ ...tag, kind }));
}

/**
 * updateTags は `ids` の行を `update` で書き換える。並び順の値を変えない書き換えに使い、
 * 行の位置はそのままで、今の条件に合わなくなった行は取り除いて `total` を減らす。
 * 行の数によらず 1 回の走査で済ませる。`update` が同じ行を返したら何もしない。
 */
function updateTags(
  page: TagPageRows,
  ids: ReadonlySet<number>,
  update: (tag: Tag) => Tag,
): TagPageRows {
  if (ids.size === 0) return page;
  let removed = 0;
  const rows: Tag[] = [];
  for (const tag of page.rows) {
    const next = ids.has(tag.id) ? update(tag) : tag;
    if (next === tag) {
      rows.push(tag);
      continue;
    }
    if (matchesTagQuery(next, page.query)) rows.push(next);
    else removed += 1;
  }
  return { ...page, rows, total: Math.max(0, page.total - removed) };
}

/**
 * addTag は作ったタグを反映する。今の条件に合えば並び順の位置へ置き `total` を
 * 増やし、合わなければ `totalAll` だけを増やす。
 */
export function addTag(page: TagPageRows, created: Tag): TagPageRows {
  return addRow(page, created, matchesTagQuery);
}

/** addRow は `addTag` の、条件の判定（`matches`）を受け取る形である。 */
export function addRow<R extends ListRow, Q extends ListRowsQuery>(
  page: PageRows<R, Q>,
  created: R,
  matches: RowMatcher<R, Q>,
): PageRows<R, Q> {
  const next = replaceRow(page, undefined, created, matches);
  return { ...next, totalAll: page.totalAll + 1 };
}
