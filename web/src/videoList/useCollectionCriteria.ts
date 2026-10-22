import { useCallback, useMemo, useRef } from "react";

import type { CollectionCriteria } from "../api/collectionCriteria";
import type { VideoSort } from "../api/client";
import {
  criteriaKey,
  hasConditions,
  type HistoryMode,
  type ListCriteriaChange,
  newSeed,
} from "./listCriteria";
import { useListActions } from "./useListActions";
import { useListCriteria } from "./useListCriteria";
import { useScrollTopOnChange } from "./useScrollTopOnChange";

const trailParams = ["trail"];

/** Temporary list conditions never modify the collection's saved rules or playback order. */
export function useCollectionCriteria() {
  const { criteria, apply, nativeOrder } = useListCriteria(
    "addedDesc",
    trailParams,
    trailParams,
    true,
  );
  const searchRef = useRef<HTMLInputElement>(null);
  const update = useCallback(
    (next: ListCriteriaChange, mode: HistoryMode = "push") => apply(next, mode),
    [apply],
  );
  const actions = useListActions(update);
  const changeSort = useCallback(
    (sort: VideoSort) =>
      apply(
        (current) => ({
          ...current,
          sort,
          seed: sort === "random" ? newSeed(current.seed) : undefined,
        }),
        "push",
        undefined,
        false,
      ),
    [apply],
  );
  const savedOrder = useCallback(
    () =>
      apply(
        (current) => ({ ...current, sort: "addedDesc", seed: undefined }),
        "push",
        undefined,
        true,
      ),
    [apply],
  );
  const key = `${nativeOrder}:${criteriaKey(criteria)}`;
  useScrollTopOnChange(key);
  const conditions = useMemo<CollectionCriteria | undefined>(() => {
    if (!criteria.favorite && nativeOrder) return undefined;
    return {
      favorite: criteria.favorite,
      sort: nativeOrder ? undefined : criteria.sort,
      seed: criteria.seed,
    };
  }, [criteria.favorite, criteria.sort, criteria.seed, nativeOrder]);
  return {
    criteria,
    conditions,
    key,
    filtered: hasConditions(criteria),
    customOrder: !nativeOrder,
    toolbar: {
      query: criteria.query,
      onQueryCommit: actions.commitQuery,
      searchRef,
      sort: criteria.sort,
      onSortChange: changeSort,
      onShuffle: actions.shuffle,
      favorite: criteria.favorite,
      onFavoriteChange: actions.changeFavorite,
      canClear: hasConditions(criteria),
      onClear: actions.clearAll,
      nativeOrder: { active: nativeOrder, onSelect: savedOrder },
    },
  };
}
