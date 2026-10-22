import type { CreditKind } from "../api/credits";
import { withCredit, withTagKind } from "./creditCriteria";
import type { TagOrKindPick } from "./tagChoices";
import { addTagId, MAX_TAG_COUNT, serializeTagIds, TAG_PARAM } from "./tagCriteria";

/** One filter chosen from the filter popover's pickers or the search suggestions. */
export type LibraryFilterPick = TagOrKindPick | { kind: CreditKind; id: number };

export type PickParams =
  | { kind: "params"; params: Record<string, string[]> }
  /** The tag is already in the filter. */
  | { kind: "unchanged" }
  | { kind: "tagLimit" };

/**
 * The URL parameters a pick writes: a tag joins the AND list up to its limit, while a kind, person
 * or organization replaces the one held.
 */
export function pickParams(
  pick: LibraryFilterPick,
  tagIds: readonly number[],
): PickParams {
  switch (pick.kind) {
    case "tag":
      if (tagIds.includes(pick.id)) return { kind: "unchanged" };
      if (tagIds.length >= MAX_TAG_COUNT) return { kind: "tagLimit" };
      return {
        kind: "params",
        params: { [TAG_PARAM]: serializeTagIds(addTagId(tagIds, pick.id)) },
      };
    case "tagKind":
      return { kind: "params", params: withTagKind(pick.id) };
    default:
      return { kind: "params", params: withCredit(pick.kind, pick.id) };
  }
}
