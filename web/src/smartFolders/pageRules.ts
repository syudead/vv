import type { LibraryItem } from "../api/client";
import type { GroupRef } from "../api/groups";
import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import { fromSelectAll, type LibrarySelection } from "../library/selection";
import { conditionChips, hasMissing } from "./conditions";

/**
 * missing means a condition references a deleted target; both missing and empty follow child-
 * folder cards.
 */
export type BodyKind = "content" | "empty" | "missing";

/**
 * Any content or continuation wins over missing/empty; only owner responses expose missing
 * references.
 */
export function bodyKind(
  items: readonly LibraryItem[],
  nextCursor: string | undefined,
  missing: readonly SmartRuleRef[],
): BodyKind {
  const filled = items.some((item) => item.kind !== "smartFolder");
  if (filled || nextCursor !== undefined) return "content";
  return missing.length > 0 ? "missing" : "empty";
}

export interface AncestorRule {
  folder: GroupRef;
  rule?: SmartRule;
}

/**
 * Return the nearest ancestor with missing references only when this folder has none; ancestors
 * are nearest-first.
 */
export function missingParent(
  own: SmartRule | undefined,
  ancestors: readonly AncestorRule[],
  missing: readonly SmartRuleRef[],
): GroupRef | undefined {
  if (own === undefined || hasMissing(conditionChips(own, missing))) return undefined;
  return ancestors.find(
    (ancestor) =>
      ancestor.rule !== undefined && hasMissing(conditionChips(ancestor.rule, missing)),
  )?.folder;
}

/** Select loaded videos, all members of loaded groups and loaded child-folder cards. */
export function selectAllLoaded(items: readonly LibraryItem[]): LibrarySelection {
  const ids: number[] = [];
  const groups = [];
  const smartFolderIds: number[] = [];
  for (const item of items) {
    if (item.kind === "video") ids.push(item.video.id);
    if (item.kind === "smartFolder") smartFolderIds.push(item.smartFolder.id);
    if (item.kind === "group") {
      ids.push(...item.group.videoIds);
      groups.push({ id: item.group.id, videoIds: item.group.videoIds });
    }
  }
  return fromSelectAll(ids, groups, smartFolderIds);
}
