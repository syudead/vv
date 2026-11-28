import { useCallback, useRef } from "react";

import {
  DEFAULT_SORT,
  hasConditions,
  type HistoryMode,
  type ListCriteriaChange,
} from "./listCriteria";
import type { ShelfToolbarProps } from "./ShelfToolbar";
import { useListActions } from "./useListActions";
import { useListCriteria } from "./useListCriteria";

const noParams: readonly string[] = [];

/**
 * The search, sort and favorites filter of a record page's video shelves, in its URL as the
 * library's are. The sort starts at the library's default order, not the stored preference, and a
 * change keeps the page's history state.
 */
export function useShelfCriteria() {
  const { criteria, apply } = useListCriteria(
    DEFAULT_SORT,
    noParams,
    noParams,
    false,
    true,
  );
  const searchRef = useRef<HTMLInputElement>(null);
  const update = useCallback(
    (next: ListCriteriaChange, mode: HistoryMode = "push") => apply(next, mode),
    [apply],
  );
  const actions = useListActions(update);
  const filtered = hasConditions(criteria);
  const toolbar: ShelfToolbarProps = {
    query: criteria.query,
    onQueryCommit: actions.commitQuery,
    searchRef,
    sort: criteria.sort,
    onSortChange: actions.changeSort,
    onShuffle: actions.shuffle,
    favorite: criteria.favorite,
    onFavoriteChange: actions.changeFavorite,
    canClear: filtered,
    onClear: actions.clearAll,
  };
  return { criteria, filtered, toolbar };
}

export type ShelfCriteria = ReturnType<typeof useShelfCriteria>;
