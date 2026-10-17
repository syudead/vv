import type { VideoSort, WatchFilter } from "../api/client";
import type { SmartFolderSummary, SmartRule } from "../api/smartFolders";
import { compareNatural } from "../api/tagOrder";
import { type AddedLengthFilter, validAddedLength } from "../library/addedLengthCriteria";
import type { CreditFilter } from "../library/creditCriteria";
import { DEFAULT_SORT, type ListCriteria } from "../videoList/listCriteria";

/**
 * smartRule はスマートフォルダの条件（SmartRule）と、ライブラリの絞り込み・親の木との間の
 * 純粋な変換である。
 */

/** SMART_RULE_MAX_QUERY は条件の検索語の長さの上限（符号位置）である。 */
export const SMART_RULE_MAX_QUERY = 500;
/** SMART_RULE_MAX_TAGS は条件のタグの数の上限である。 */
export const SMART_RULE_MAX_TAGS = 16;

/** LibraryFilter はライブラリの今の絞り込みの全体である。 */
export interface LibraryFilter {
  criteria: ListCriteria;
  tagIds: readonly number[];
  credits: CreditFilter;
  addedLength: AddedLengthFilter;
}

/**
 * cleanRule は空の条件を落とした条件を返す（検索語の空、`watch` の `all`、偽の
 * チェック、空のタグ、無い id）。空の条件は送らない。
 */
export function cleanRule(rule: SmartRule): SmartRule {
  const clean: SmartRule = {};
  if (rule.q !== undefined && rule.q !== "") clean.q = rule.q;
  if (rule.watch !== undefined) clean.watch = rule.watch;
  if (rule.favorite === true) clean.favorite = true;
  if (rule.playable === true) clean.playable = true;
  if (rule.tagKind !== undefined) clean.tagKind = rule.tagKind;
  if (rule.tags !== undefined && rule.tags.length > 0) clean.tags = [...rule.tags];
  if (rule.person !== undefined) clean.person = rule.person;
  if (rule.organization !== undefined) clean.organization = rule.organization;
  if (rule.addedWithin !== undefined) clean.addedWithin = rule.addedWithin;
  if (rule.minDuration !== undefined) clean.minDuration = rule.minDuration;
  if (rule.maxDuration !== undefined) clean.maxDuration = rule.maxDuration;
  return clean;
}

/** ruleWatch は条件の視聴状態をライブラリの視聴状態にする（無ければ `all`）。 */
export function ruleWatch(rule: SmartRule): WatchFilter {
  return rule.watch ?? "all";
}

/** withWatch は視聴状態を差し替えた条件を返す（`all` は条件から外す）。 */
export function withWatch(rule: SmartRule, watch: WatchFilter): SmartRule {
  return { ...rule, watch: watch === "all" ? undefined : watch };
}

/**
 * ruleFromLibrary はライブラリの今の絞り込みを条件にする。並べ順は含めない（folderSort が別に決める）。
 */
export function ruleFromLibrary(filter: LibraryFilter): SmartRule {
  const { criteria, tagIds, credits, addedLength } = filter;
  return cleanRule({
    q: criteria.query,
    watch: criteria.watch === "all" ? undefined : criteria.watch,
    favorite: criteria.favorite,
    playable: criteria.playable,
    tagKind: credits.tagKind,
    tags: [...tagIds],
    person: credits.person,
    organization: credits.organization,
    addedWithin: addedLength.addedWithin,
    minDuration: addedLength.minDuration,
    maxDuration: addedLength.maxDuration,
  });
}

/** hasRuleMember は条件が 1 つでもあるかを返す。 */
export function hasRuleMember(rule: SmartRule): boolean {
  return Object.keys(cleanRule(rule)).length > 0;
}

function positiveId(id: number | undefined): boolean {
  return id === undefined || (Number.isSafeInteger(id) && id > 0);
}

function validTags(tags: readonly number[] | undefined): boolean {
  if (tags === undefined) return true;
  if (tags.length === 0 || tags.length > SMART_RULE_MAX_TAGS) return false;
  return tags.every(positiveId) && new Set(tags).size === tags.length;
}

/**
 * validRule は条件をサーバーが受け付けるかを返す（domain.SmartRule.Validate と同じ範囲）。
 * 範囲の外の条件は送らずに条件の欄の誤りを出す。
 */
export function validRule(rule: SmartRule): boolean {
  const clean = cleanRule(rule);
  if (Array.from(clean.q ?? "").length > SMART_RULE_MAX_QUERY) return false;
  if (!validTags(clean.tags)) return false;
  if (![clean.tagKind, clean.person, clean.organization].every(positiveId)) return false;
  return validAddedLength(clean);
}

/** ruleKey は条件を値で比べるための文字列にする（空の条件を落とし、鍵の順をそろえる）。 */
export function ruleKey(rule: SmartRule): string {
  const clean = cleanRule(rule);
  return JSON.stringify(
    Object.keys(clean)
      .sort()
      .map((key) => [key, clean[key as keyof SmartRule]]),
  );
}

/**
 * folderSort はフォルダに保存する並べ順を返す。ランダムは保存できないので追加日の新しい
 * 順にし、`randomReplaced` で知らせる。
 */
export function folderSort(sort: VideoSort): {
  sort: VideoSort;
  randomReplaced: boolean;
} {
  return sort === "random"
    ? { sort: DEFAULT_SORT, randomReplaced: true }
    : { sort, randomReplaced: false };
}

/** ParentChoice は親の `Select` の 1 行である（深さは字下げの段）。 */
export interface ParentChoice {
  folder: SmartFolderSummary;
  depth: number;
}

function childrenByParent(
  folders: readonly SmartFolderSummary[],
): Map<number | undefined, SmartFolderSummary[]> {
  const known = new Set(folders.map((folder) => folder.id));
  const byParent = new Map<number | undefined, SmartFolderSummary[]>();
  for (const folder of folders) {
    // 見えない親（ゲストなど）の子は一番上に置く。
    const parent =
      folder.parentId !== undefined && known.has(folder.parentId)
        ? folder.parentId
        : undefined;
    const siblings = byParent.get(parent) ?? [];
    siblings.push(folder);
    byParent.set(parent, siblings);
  }
  for (const siblings of byParent.values()) {
    siblings.sort((a, b) => compareNatural(a.name, b.name));
  }
  return byParent;
}

/**
 * descendantIds は `id` の子孫（どの深さも）の id を返す。`id` 自身は入らない。
 */
export function descendantIds(
  folders: readonly SmartFolderSummary[],
  id: number,
): Set<number> {
  const byParent = childrenByParent(folders);
  const found = new Set<number>();
  const stack = [...(byParent.get(id) ?? [])];
  while (stack.length > 0) {
    const folder = stack.pop() as SmartFolderSummary;
    if (found.has(folder.id)) continue;
    found.add(folder.id);
    stack.push(...(byParent.get(folder.id) ?? []));
  }
  return found;
}

/**
 * parentChoices は親の `Select` に並べるフォルダを、木の順（親の直後にその子を自然な名前の
 * 順）と深さで返す。`editing` を渡すと、そのフォルダと子孫を除く — 選べば自分の中へ入る
 * ことになるから。
 */
export function parentChoices(
  folders: readonly SmartFolderSummary[],
  editing?: number,
): ParentChoice[] {
  const byParent = childrenByParent(folders);
  const choices: ParentChoice[] = [];
  const visit = (parent: number | undefined, depth: number) => {
    for (const folder of byParent.get(parent) ?? []) {
      if (folder.id === editing) continue;
      choices.push({ folder, depth });
      visit(folder.id, depth + 1);
    }
  };
  visit(undefined, 0);
  return choices;
}
