import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";
import { useLocation } from "react-router";

import { type TagRef, videoConditionNames } from "../api/client";
import type { CreditKind } from "../api/credits";
import { refreshTags, revalidateTags } from "../api/tags";
import { t } from "../i18n";
import { useToast } from "../ui/Toast";
import type { HistoryMode, ListCriteria } from "../videoList/listCriteria";
import type { ListCriteriaState } from "../videoList/useListCriteria";
import ActiveTagFilters from "./ActiveTagFilters";
import {
  clearedCredits,
  type CreditFilter,
  creditKinds,
  creditSignature,
  hasFilterChips,
  ORGANIZATION_PARAM,
  PERSON_PARAM,
  TAG_KIND_PARAM,
  useCreditFilter,
  withoutCredit,
} from "./creditCriteria";
import { CreditFilterField, TagFilterField } from "./FilterFields";
import { type LibraryFilterPick, pickParams } from "./filterPicks";
import { useSearchSuggestions } from "./searchSuggestions";
import {
  clearConditions as clearTagConditions,
  hasConditions as hasTagConditions,
  MAX_TAG_COUNT,
  parseTagParam,
  removeTagId,
  serializeTagIds,
  TAG_PARAM,
} from "./tagCriteria";
import { draftParams, useFilterDraft } from "./useFilterDraft";
import { type ApplyTagKind, useLibraryTagKinds } from "./useLibraryTagKinds";
import {
  clearedVideoConditions,
  hasVideoConditions,
  useVideoConditions,
  type VideoConditionKind,
  videoConditionChips,
  type VideoConditions,
  videoConditionsSignature,
  withoutVideoCondition,
} from "./videoConditions";

/** URL parameters useListCriteria must keep for the tag, kind and credit filters. */
export const libraryFilterParams = [
  TAG_PARAM,
  PERSON_PARAM,
  ORGANIZATION_PARAM,
  TAG_KIND_PARAM,
];

/** The library also keeps the video conditions, which folder listings do not take. */
export const libraryPageParams = [...libraryFilterParams, ...videoConditionNames];

const noConditions: VideoConditions = {};

/** Each tag, the kind, the person, the organization and each condition chip count as one applied filter. */
function idFilterCount(
  tagIds: readonly number[],
  credits: CreditFilter,
  conditions: VideoConditions,
): number {
  return (
    tagIds.length +
    [credits.tagKind, credits.person, credits.organization].filter(
      (id) => id !== undefined,
    ).length +
    videoConditionChips(conditions).length
  );
}

/** The tag, credit and video-condition filters in the URL; guests get none. */
export function useLibraryFilterValues(owner: boolean) {
  const location = useLocation();
  // Preserve tag-array identity across unrelated URL changes so card callbacks and memoization remain stable.
  const rawTagKey = JSON.stringify(
    new URLSearchParams(location.search).getAll(TAG_PARAM),
  );
  const tagIds = useMemo(
    () => (owner ? parseTagParam(JSON.parse(rawTagKey) as string[]) : []),
    [owner, rawTagKey],
  );
  const credits = useCreditFilter(location.search, owner);
  const conditions = useVideoConditions(location.search, owner);
  return { tagIds, credits, conditions };
}

interface Options {
  owner: boolean;
  criteria: ListCriteria;
  apply: ListCriteriaState["apply"];
  tagIds: readonly number[];
  credits: CreditFilter;
  /** Omitted on folder pages, whose listing does not take them. */
  conditions?: VideoConditions;
  /** Tags the last response dropped as deleted; they leave the URL once per response. */
  missingTagIds: readonly number[];
  /** Whether the list came from a snapshot, which skips the request that reports deleted tags. */
  restored: boolean;
  resetPreview: () => void;
  searchField: RefObject<HTMLInputElement | null>;
}

/**
 * The library's tag, tag-kind, person and organization filters: the toolbar's filter fields and
 * suggestions, the active filter chips, clearing, and removal of filters that no longer exist.
 */
export function useLibraryFilters({
  owner,
  criteria,
  apply,
  tagIds,
  credits,
  conditions = noConditions,
  missingTagIds,
  restored,
  resetPreview,
  searchField,
}: Options) {
  const toast = useToast();
  const clearAll = useCallback(() => {
    resetPreview();
    const cleared = clearTagConditions(criteria);
    apply(cleared.criteria, "push", {
      [TAG_PARAM]: serializeTagIds(cleared.tag),
      ...clearedCredits,
      ...clearedVideoConditions,
    });
  }, [apply, criteria, resetPreview]);

  // Read current criteria through refs to keep card callbacks stable across unrelated renders.
  const latestConditions = useRef({ criteria, tagIds });
  latestConditions.current = { criteria, tagIds };
  const applyTags = useCallback(
    (next: ListCriteria, mode: HistoryMode, ids: readonly number[]) =>
      apply(next, mode, { [TAG_PARAM]: serializeTagIds(ids) }),
    [apply],
  );

  const removeCredit = useCallback(
    (kind: CreditKind) => {
      resetPreview();
      apply(latestConditions.current.criteria, "push", withoutCredit(kind));
    },
    [apply, resetPreview],
  );
  const removeCondition = useCallback(
    (kind: VideoConditionKind) => {
      resetPreview();
      apply(latestConditions.current.criteria, "push", withoutVideoCondition(kind));
    },
    [apply, resetPreview],
  );
  // Remove missing credits with history replacement and notify; changed criteria reload the list.
  const creditGone = useCallback(
    (kind: CreditKind) => {
      toast(
        kind === "person"
          ? t.library.activeTags.personGone
          : t.library.activeTags.organizationGone,
      );
      apply(latestConditions.current.criteria, "replace", withoutCredit(kind));
    },
    [apply, toast],
  );
  const applyTagKind = useCallback<ApplyTagKind>(
    (extra, mode) => {
      resetPreview();
      apply(latestConditions.current.criteria, mode, extra);
    },
    [apply, resetPreview],
  );
  const tagKinds = useLibraryTagKinds(credits.tagKind, applyTagKind);
  const changeTagKind = tagKinds.change;
  const removeTagKind = useCallback(() => changeTagKind(undefined), [changeTagKind]);

  // A search suggestion also clears the typed text, in the same history entry as the filter.
  const applyPick = useCallback(
    (pick: LibraryFilterPick, clearQuery = false) => {
      const { criteria: current, tagIds: currentTagIds } = latestConditions.current;
      const result = pickParams(pick, currentTagIds);
      if (result.kind === "tagLimit") {
        toast(t.errors.reason.too_many_tag_filters({ limit: MAX_TAG_COUNT }));
      }
      if (result.kind !== "params" && !clearQuery) return;
      resetPreview();
      apply(
        clearQuery ? { ...current, query: "" } : current,
        "push",
        result.kind === "params" ? result.params : undefined,
      );
    },
    [apply, resetPreview, toast],
  );
  const pressTag = useCallback(
    (tag: TagRef) => applyPick({ kind: "tag", id: tag.id }),
    [applyPick],
  );
  const pickSuggestion = useCallback(
    (pick: LibraryFilterPick) => applyPick(pick, true),
    [applyPick],
  );
  const suggestions = useSearchSuggestions(owner, pickSuggestion);
  const filterDraft = useFilterDraft(tagIds, credits);
  const resetFilterDraft = filterDraft.reset;
  const loadTagKinds = tagKinds.load;
  const openFilter = useCallback(() => {
    resetFilterDraft();
    if (owner) loadTagKinds();
  }, [loadTagKinds, owner, resetFilterDraft]);
  const addDraftTag = (id: number) => {
    if (!filterDraft.addTag(id)) {
      toast(t.errors.reason.too_many_tag_filters({ limit: MAX_TAG_COUNT }));
    }
  };
  const applyFilter = (favorite: boolean) => {
    resetPreview();
    apply(
      { ...latestConditions.current.criteria, favorite },
      "push",
      draftParams(filterDraft.draft),
    );
  };
  const removeActiveTag = useCallback(
    (id: number) => {
      resetPreview();
      applyTags(criteria, "push", removeTagId(tagIds, id));
    },
    [applyTags, criteria, resetPreview, tagIds],
  );

  // Remove missing tag filters once per response content; URL replacement can cause repeated intermediate renders.
  const handledMissingTagIds = useRef<string | null>(null);
  useEffect(() => {
    if (missingTagIds.length === 0) {
      handledMissingTagIds.current = null;
      return;
    }
    const signature = [...missingTagIds].sort((a, b) => a - b).join(",");
    if (handledMissingTagIds.current === signature) return;
    handledMissingTagIds.current = signature;
    const remaining = tagIds.filter((id) => !missingTagIds.includes(id));
    if (remaining.length === tagIds.length) return;
    toast(t.library.deletedTagsRemoved);
    refreshTags().catch(() => undefined);
    applyTags(criteria, "replace", remaining);
  }, [applyTags, criteria, missingTagIds, tagIds, toast]);

  // Restored snapshots skip list requests, so validate URL tags against the refreshed shared tag list on mount.
  const mountRef = useRef({ criteria, tagIds, apply: applyTags, toast });
  mountRef.current = { criteria, tagIds, apply: applyTags, toast };
  useEffect(() => {
    if (!restored || mountRef.current.tagIds.length === 0) return;
    let alive = true;
    revalidateTags()
      .then((tags) => {
        if (!alive) return;
        const known = new Set(tags.map((tag) => tag.id));
        const {
          tagIds: current,
          criteria: currentCriteria,
          apply: currentApply,
        } = mountRef.current;
        const remaining = current.filter((id) => known.has(id));
        if (remaining.length === current.length) return;
        mountRef.current.toast(t.library.deletedTagsRemoved);
        currentApply(currentCriteria, "replace", remaining);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [restored]);

  const toolbar = {
    onFilterApply: applyFilter,
    extraFilter: (
      <>
        <TagFilterField
          tagIds={filterDraft.draft.tagIds}
          kindId={filterDraft.draft.tagKind}
          kinds={tagKinds.kinds}
          maxTags={MAX_TAG_COUNT}
          onAddTag={addDraftTag}
          onRemoveTag={filterDraft.removeTag}
          onKindChange={(tagKind) => filterDraft.change({ tagKind })}
        />
        {creditKinds.map((kind) => (
          <CreditFilterField
            key={kind}
            kind={kind}
            value={filterDraft.draft[kind]}
            onChange={(id) => filterDraft.change({ [kind]: id })}
          />
        ))}
      </>
    ),
    extraFilterCount: idFilterCount(tagIds, credits, conditions),
    onFilterOpen: openFilter,
    suggestions,
  };

  const activeFilters =
    hasFilterChips(tagIds, credits) || hasVideoConditions(conditions) ? (
      <ActiveTagFilters
        tagIds={tagIds}
        onRemove={removeActiveTag}
        credits={credits}
        onRemoveCredit={removeCredit}
        onCreditGone={creditGone}
        tagKindName={tagKinds.name}
        onRemoveTagKind={removeTagKind}
        conditions={conditions}
        onRemoveCondition={removeCondition}
        searchFieldRef={searchField}
      />
    ) : null;

  return {
    conditioned: hasTagConditions(criteria, tagIds, credits, conditions),
    signature: [
      tagIds.join(","),
      creditSignature(credits),
      videoConditionsSignature(conditions),
    ].join("\u0000"),
    clearAll,
    pressTag,
    toolbar,
    activeFilters,
  };
}
