import { useCallback, useRef } from "react";
import { useSearchParams } from "react-router";

import { type HistoryMode, normalizeQuery } from "./listCriteria";
import { useScrollTopOnChange } from "./useScrollTopOnChange";

/** Search the current collection without changing its identity, trail or saved settings. */
export function useSearchQuery() {
  const [params, setParams] = useSearchParams();
  const query = normalizeQuery(params.get("q") ?? "");
  const searchRef = useRef<HTMLInputElement>(null);
  const commitQuery = useCallback(
    (next: string, mode: HistoryMode) => {
      setParams(
        (current) => {
          const updated = new URLSearchParams(current);
          if (next === "") updated.delete("q");
          else updated.set("q", next);
          return updated;
        },
        { replace: mode === "replace" },
      );
    },
    [setParams],
  );
  useScrollTopOnChange(query);
  return { query, commitQuery, searchRef };
}
