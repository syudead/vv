import { useMemo } from "react";

import type { CreditKind } from "../api/credits";

/**
 * creditCriteria は、ライブラリだけが持つ人物・団体の絞り込み（URL の `person` と
 * `organization`）の読み書きを行う。tagCriteria と同じく、共有の `ListCriteria` には足さない —
 * 足すとフォルダ画面も読んで URL に残してしまい、フォルダ画面の URL を変えない約束に反する。
 */

/** URL の `person` パラメータの名前。 */
export const PERSON_PARAM = "person";

/** URL の `organization` パラメータの名前。 */
export const ORGANIZATION_PARAM = "organization";

/**
 * URL の `tagKind` パラメータの名前。人物・団体と同じくライブラリだけの条件なので、ここで
 * 一緒に読み書きする。
 */
export const TAG_KIND_PARAM = "tagKind";

/** creditKinds は絞り込みのチップを並べる順（人物、団体）である。 */
export const creditKinds: readonly CreditKind[] = ["person", "organization"];

/**
 * CreditFilter は人物 1 つと団体 1 つまで、それにタグの種類 1 つまでの絞り込みである。
 * どれも id 1 つで、一覧の要求・控えの鍵・「Clear filters」を同じ形で通る。
 */
export interface CreditFilter {
  person?: number;
  organization?: number;
  tagKind?: number;
}

/** creditParam は種類の URL のパラメータの名前を返す。 */
function creditParam(kind: CreditKind): string {
  return kind === "person" ? PERSON_PARAM : ORGANIZATION_PARAM;
}

/**
 * parseCreditParam は URL の生の値（`getAll` の結果）を id にする。正の整数でない値は
 * 捨て、いくつもあれば最初の正しい値を使う（1 つまで）。誤りは出さない。
 */
export function parseCreditParam(raw: readonly string[]): number | undefined {
  for (const value of raw) {
    if (!/^\d+$/.test(value)) continue;
    const id = Number(value);
    if (Number.isSafeInteger(id) && id > 0) return id;
  }
  return undefined;
}

/** parseCreditFilter は URL の問い合わせ部分から人物・団体の絞り込みを読む。 */
export function parseCreditFilter(search: URLSearchParams): CreditFilter {
  return {
    person: parseCreditParam(search.getAll(PERSON_PARAM)),
    organization: parseCreditParam(search.getAll(ORGANIZATION_PARAM)),
    tagKind: parseCreditParam(search.getAll(TAG_KIND_PARAM)),
  };
}

/** hasCreditFilter は人物か団体かタグの種類で絞り込んでいるかを返す。 */
export function hasCreditFilter(filter: CreditFilter): boolean {
  return (
    filter.person !== undefined ||
    filter.organization !== undefined ||
    filter.tagKind !== undefined
  );
}

/**
 * useCreditFilter は URL の問い合わせ部分から人物・団体の絞り込みを読み、値が変わらない
 * 限り同じオブジェクトを返す。ゲストは絞り込めないので常に空（URL に残った値は
 * useListCriteria が取り除く）。
 */
export function useCreditFilter(search: string, owner: boolean): CreditFilter {
  const parsed = parseCreditFilter(new URLSearchParams(search));
  const person = owner ? parsed.person : undefined;
  const organization = owner ? parsed.organization : undefined;
  const tagKind = owner ? parsed.tagKind : undefined;
  return useMemo(
    () => ({ person, organization, tagKind }),
    [person, organization, tagKind],
  );
}

/** creditSignature は絞り込みを値で比べるための文字列にする。 */
export function creditSignature(filter: CreditFilter): string {
  return [filter.person, filter.organization, filter.tagKind]
    .map((id) => String(id ?? ""))
    .join("\u0000");
}

/**
 * hasFilterChips は、絞り込み中の条件の行に出すチップ（タグ・人物・団体・タグの種類）が
 * あるかを返す。
 */
export function hasFilterChips(tagIds: readonly number[], filter: CreditFilter): boolean {
  return tagIds.length > 0 || hasCreditFilter(filter);
}

/**
 * withoutCredit は、その種類のパラメータだけを外す `apply` の追加の値である。
 * タグともう一方の種類はそのまま残る。
 */
export function withoutCredit(kind: CreditKind): Record<string, string[]> {
  return { [creditParam(kind)]: [] };
}

/** withoutTagKind はタグの種類の絞り込みだけを外す `apply` の追加の値である。 */
export const withoutTagKind: Readonly<Record<string, string[]>> = {
  [TAG_KIND_PARAM]: [],
};

/** withTagKind はタグの種類の絞り込みを `id` に差し替える `apply` の追加の値である。 */
export function withTagKind(id: number): Record<string, string[]> {
  return { [TAG_KIND_PARAM]: [String(id)] };
}

/** clearedCredits は「Clear filters」で人物・団体・タグの種類を外す `apply` の追加の値である。 */
export const clearedCredits: Readonly<Record<string, string[]>> = {
  [PERSON_PARAM]: [],
  [ORGANIZATION_PARAM]: [],
  [TAG_KIND_PARAM]: [],
};
