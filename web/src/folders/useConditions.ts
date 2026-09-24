import { useCallback, useRef, useState } from "react";

import type { VideoSort } from "../api/client";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import {
  clearConditions,
  type HistoryMode,
  type ListCriteria,
  newSeed,
} from "../videoList/listCriteria";
import { useListCriteria } from "../videoList/useListCriteria";

/**
 * useConditions は一覧の条件（検索語・視聴状態・再生可否・並べ替え・seed）を
 * URL から読み書きする口である。ライブラリと同じ `videoList/listCriteria`・
 * `useListCriteria` を使う（Plan の Structural Decisions 9）。端末に保存するのは
 * 並べ替えだけで、フォルダ画面もライブラリと同じ保存値を読み書きする。
 */
export function useConditions() {
  const [preferences, setPreferences] = useState(readViewPreferences);
  const { criteria, apply } = useListCriteria(preferences.sort);
  const searchField = useRef<HTMLInputElement | null>(null);

  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  const update = useCallback(
    (next: ListCriteria, mode: HistoryMode = "push") => apply(next, mode),
    [apply],
  );

  const changeSort = useCallback(
    (next: VideoSort) => {
      update({
        ...criteria,
        sort: next,
        seed: next === "random" ? newSeed(criteria.seed) : undefined,
      });
      savePreferences({ ...preferences, sort: next });
    },
    [criteria, preferences, savePreferences, update],
  );
  const shuffle = useCallback(
    () => update({ ...criteria, seed: newSeed(criteria.seed) }),
    [criteria, update],
  );
  const changeWatch = useCallback(
    (next: ListCriteria["watch"]) => update({ ...criteria, watch: next }),
    [criteria, update],
  );
  const changePlayable = useCallback(
    (next: boolean) => update({ ...criteria, playable: next }),
    [criteria, update],
  );
  const commitQuery = useCallback(
    (next: string, mode: HistoryMode) => update({ ...criteria, query: next }, mode),
    [criteria, update],
  );
  const clearAll = useCallback(
    () => update(clearConditions(criteria)),
    [criteria, update],
  );
  const changeZoom = useCallback(
    (next: Zoom) => savePreferences({ ...preferences, zoom: next }),
    [preferences, savePreferences],
  );

  return {
    criteria,
    zoom: preferences.zoom,
    searchField,
    changeSort,
    shuffle,
    changeWatch,
    changePlayable,
    commitQuery,
    clearAll,
    changeZoom,
  };
}
