import { useCallback } from "react";

import {
  clearConditions,
  type HistoryMode,
  type ListCriteriaChange,
  newSeed,
} from "./listCriteria";

export function useListActions(
  update: (next: ListCriteriaChange, mode?: HistoryMode) => void,
) {
  const changeFavorite = useCallback(
    (favorite: boolean) => update((current) => ({ ...current, favorite })),
    [update],
  );
  const commitQuery = useCallback(
    (query: string, mode: HistoryMode) =>
      update((current) => ({ ...current, query }), mode),
    [update],
  );
  const shuffle = useCallback(
    () => update((current) => ({ ...current, seed: newSeed(current.seed) })),
    [update],
  );
  const clearAll = useCallback(() => update(clearConditions), [update]);
  return { changeFavorite, commitQuery, shuffle, clearAll };
}
