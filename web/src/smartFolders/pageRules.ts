import type { LibraryItem } from "../api/client";
import type { GroupRef } from "../api/groups";
import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
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

/** Loaded videos and all members of loaded groups; the selection may hold only these. */
export function loadedVideoIds(items: readonly LibraryItem[]): ReadonlySet<number> {
  const ids = new Set<number>();
  for (const item of items) {
    if (item.kind === "video") ids.add(item.video.id);
    if (item.kind === "group") for (const id of item.group.videoIds) ids.add(id);
  }
  return ids;
}
