import { useCallback, useState } from "react";

import type { CreditKind } from "../api/credits";
import {
  type CreditFilter,
  withCredit,
  withoutCredit,
  withoutTagKind,
  withTagKind,
} from "./creditCriteria";
import {
  addTagId,
  MAX_TAG_COUNT,
  removeTagId,
  serializeTagIds,
  TAG_PARAM,
} from "./tagCriteria";

/** The library filter popover's fields, edited apart from the URL until Apply. */
export interface FilterDraft {
  tagIds: readonly number[];
  tagKind?: number;
  person?: number;
  organization?: number;
}

/** The URL parameters that replace every drafted condition, clearing the ones left empty. */
export function draftParams(draft: FilterDraft): Record<string, string[]> {
  const credit = (kind: CreditKind) => {
    const id = draft[kind];
    return id === undefined ? withoutCredit(kind) : withCredit(kind, id);
  };
  return {
    [TAG_PARAM]: serializeTagIds(draft.tagIds),
    ...(draft.tagKind === undefined ? withoutTagKind : withTagKind(draft.tagKind)),
    ...credit("person"),
    ...credit("organization"),
  };
}

/**
 * Holds the popover's draft; `reset` copies the applied conditions in on each opening, so closing
 * without Apply drops the edits. `addTag` reports false at the tag limit.
 */
export function useFilterDraft(tagIds: readonly number[], credits: CreditFilter) {
  const [draft, setDraft] = useState<FilterDraft>({ tagIds });
  const reset = useCallback(() => {
    setDraft({
      tagIds,
      tagKind: credits.tagKind,
      person: credits.person,
      organization: credits.organization,
    });
  }, [tagIds, credits.tagKind, credits.person, credits.organization]);
  const addTag = (id: number): boolean => {
    if (draft.tagIds.includes(id)) return true;
    if (draft.tagIds.length >= MAX_TAG_COUNT) return false;
    setDraft({ ...draft, tagIds: addTagId(draft.tagIds, id) });
    return true;
  };
  const removeTag = (id: number) =>
    setDraft({ ...draft, tagIds: removeTagId(draft.tagIds, id) });
  const change = (patch: Partial<Omit<FilterDraft, "tagIds">>) =>
    setDraft({ ...draft, ...patch });
  return { draft, reset, addTag, removeTag, change };
}
