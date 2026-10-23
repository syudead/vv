import type { VideoSort } from "../api/client";
import type { SmartFolderSummary, SmartRule } from "../api/smartFolders";
import { compareNatural } from "../api/tagOrder";
import type { CreditFilter } from "../library/creditCriteria";
import { DEFAULT_SORT, type ListCriteria } from "../videoList/listCriteria";

/** Count Unicode code points. */
export const SMART_RULE_MAX_QUERY = 500;
export const SMART_RULE_MAX_TAGS = 16;

export interface LibraryFilter {
  criteria: ListCriteria;
  tagIds: readonly number[];
  credits: CreditFilter;
}

export function cleanRule(rule: SmartRule): SmartRule {
  const clean: SmartRule = {};
  if (rule.q !== undefined && rule.q !== "") clean.q = rule.q;
  if (rule.favorite === true) clean.favorite = true;
  if (rule.tagKind !== undefined) clean.tagKind = rule.tagKind;
  if (rule.tags !== undefined && rule.tags.length > 0) clean.tags = [...rule.tags];
  if (rule.person !== undefined) clean.person = rule.person;
  if (rule.organization !== undefined) clean.organization = rule.organization;
  return clean;
}

/** Sort is separate from conditions and handled by folderSort. */
export function ruleFromLibrary(filter: LibraryFilter): SmartRule {
  const { criteria, tagIds, credits } = filter;
  return cleanRule({
    q: criteria.query,
    favorite: criteria.favorite,
    tagKind: credits.tagKind,
    tags: [...tagIds],
    person: credits.person,
    organization: credits.organization,
  });
}

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

/** Match domain.SmartRule.Validate bounds; reject invalid conditions before submission. */
export function validRule(rule: SmartRule): boolean {
  const clean = cleanRule(rule);
  if (Array.from(clean.q ?? "").length > SMART_RULE_MAX_QUERY) return false;
  if (!validTags(clean.tags)) return false;
  return [clean.tagKind, clean.person, clean.organization].every(positiveId);
}

/** Omit empty conditions and stabilize key order for equality. */
export function ruleKey(rule: SmartRule): string {
  const clean = cleanRule(rule);
  return JSON.stringify(
    Object.keys(clean)
      .sort()
      .map((key) => [key, clean[key as keyof SmartRule]]),
  );
}

/** Random cannot be persisted; replace it with newest-added and set randomReplaced. */
export function folderSort(sort: VideoSort): {
  sort: VideoSort;
  randomReplaced: boolean;
} {
  return sort === "random"
    ? { sort: DEFAULT_SORT, randomReplaced: true }
    : { sort, randomReplaced: false };
}

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
    // Promote children with invisible parents to the top level.
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

/** Include every descendant depth, excluding id itself. */
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

/** Use parent-first natural name order; exclude the edited folder and descendants to prevent cycles. */
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
