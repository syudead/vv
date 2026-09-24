import { MAX_TAG_FILTER_COUNT } from "../api/client";
import {
  clearConditions as clearBaseConditions,
  hasConditions as hasBaseConditions,
  type ListCriteria,
} from "../videoList/listCriteria";

/**
 * tagCriteria は、ライブラリだけが持つタグ絞り込みの条件（URL の `tag`）の
 * 読み書きを行う（specs/014-video-tags/contracts/list-url.md §1・§2、Plan の
 * Structural Decisions 1・15）。共有の `ListCriteria`（videoList/listCriteria.ts）には
 * `tag` を足さない — 足すとフォルダ画面も `tag` を読んで URL に残してしまい、
 * フォルダ画面の URL を変えない約束に反する。
 */

/** MAX_TAG_COUNT はタグ絞り込みに使える id の最大個数である（list-url.md §1）。 */
export const MAX_TAG_COUNT = MAX_TAG_FILTER_COUNT;

/** URL の `tag` パラメータの名前。useListCriteria の画面固有パラメータの口へ渡す。 */
export const TAG_PARAM = "tag";

/**
 * parseTagParam は URL の生の `tag` の値（`getAll("tag")` の結果）を、整えた
 * id の配列にする。数でない値、重複、17 個目以降は捨てる（list-url.md §1）。
 * 誤りは出さない。
 */
export function parseTagParam(raw: readonly string[]): number[] {
  const seen = new Set<number>();
  const ids: number[] = [];
  for (const value of raw) {
    if (!/^\d+$/.test(value)) continue;
    const id = Number(value);
    if (!Number.isSafeInteger(id) || seen.has(id)) continue;
    seen.add(id);
    ids.push(id);
    if (ids.length >= MAX_TAG_COUNT) break;
  }
  return ids;
}

/**
 * serializeTagIds は id の集まりを URL に書く形にする。書く順は id の昇順に
 * そろえる（list-url.md §1、listSnapshot の鍵の一致を保つため）。
 */
export function serializeTagIds(ids: readonly number[]): string[] {
  return Array.from(new Set(ids))
    .sort((a, b) => a - b)
    .slice(0, MAX_TAG_COUNT)
    .map(String);
}

/** addTagId は今の絞り込みにタグを1つ加える。すでにあれば変わらない。上限なら変わらない。 */
export function addTagId(current: readonly number[], id: number): number[] {
  if (current.includes(id)) return [...current];
  if (current.length >= MAX_TAG_COUNT) return [...current];
  return [...current, id];
}

/** removeTagId は今の絞り込みからそのタグだけを外す。 */
export function removeTagId(current: readonly number[], id: number): number[] {
  return current.filter((existing) => existing !== id);
}

/**
 * hasConditions は、共有の hasConditions（検索語・視聴状態・再生可否）に
 * タグ絞り込みも加えた版である。「条件を解除」を出すかどうかに使う。
 */
export function hasConditions(criteria: ListCriteria, tag: readonly number[]): boolean {
  return tag.length > 0 || hasBaseConditions(criteria);
}

/**
 * clearConditions は、共有の clearConditions（検索語・視聴状態・再生可否を外す）に
 * タグ絞り込みを外すことも加えた版である。並べ替えと seed は残す。
 */
export function clearConditions(criteria: ListCriteria): {
  criteria: ListCriteria;
  tag: number[];
} {
  return { criteria: clearBaseConditions(criteria), tag: [] };
}
