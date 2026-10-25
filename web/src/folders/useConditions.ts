import { useCallback, useRef, useState } from "react";

import type { VideoSort } from "../api/client";
import {
  readViewPreferences,
  type ViewMode,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import {
  criteriaKey,
  type HistoryMode,
  type ListCriteriaChange,
  newSeed,
} from "../videoList/listCriteria";
import { useListActions } from "../videoList/useListActions";
import { useListCriteria } from "../videoList/useListCriteria";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";

const noExtraParams: readonly string[] = [];

/** Persist sort, view and card size, shared with the library; extraParams are kept in the URL. */
export function useConditions(extraParams: readonly string[] = noExtraParams) {
  const [preferences, setPreferences] = useState(readViewPreferences);
  const { criteria, apply } = useListCriteria(preferences.sort, extraParams);
  const searchField = useRef<HTMLInputElement | null>(null);
  useScrollTopOnChange(criteriaKey(criteria));

  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  const update = useCallback(
    (next: ListCriteriaChange, mode: HistoryMode = "push") => apply(next, mode),
    [apply],
  );

  const changeSort = useCallback(
    (next: VideoSort) => {
      update((current) => ({
        ...current,
        sort: next,
        seed: next === "random" ? newSeed(current.seed) : undefined,
      }));
      savePreferences({ ...preferences, sort: next });
    },
    [preferences, savePreferences, update],
  );
  const actions = useListActions(update);
  const changeZoom = useCallback(
    (next: Zoom) => savePreferences({ ...preferences, zoom: next }),
    [preferences, savePreferences],
  );
  const changeView = useCallback(
    (next: ViewMode) => savePreferences({ ...preferences, view: next }),
    [preferences, savePreferences],
  );

  return {
    criteria,
    apply,
    zoom: preferences.zoom,
    view: preferences.view,
    changeView,
    searchField,
    changeSort,
    ...actions,
    changeZoom,
  };
}
