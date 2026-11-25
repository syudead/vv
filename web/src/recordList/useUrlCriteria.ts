import { useCallback, useRef } from "react";
import { useLocation, useNavigate } from "react-router";

import type { HistoryMode } from "../videoList/listCriteria";

/** Share history rules across admin lists; retain the initially supplied serializer. */
export function useUrlCriteria<C>(
  parse: (params: URLSearchParams) => { criteria: C; hasExplicitSort: boolean },
  serialize: (criteria: C) => URLSearchParams,
): {
  criteria: C;
  apply: (change: Partial<C>, mode: HistoryMode) => void;
} {
  const location = useLocation();
  const navigate = useNavigate();
  const { criteria, hasExplicitSort } = parse(new URLSearchParams(location.search));
  const latest = useRef({ location, criteria, hasExplicitSort });
  // Remember just-written queries until the next navigation render to deduplicate same-tick updates.
  const written = useRef<string | null>(null);
  if (latest.current.location !== location) written.current = null;
  latest.current = { location, criteria, hasExplicitSort };
  const serializeRef = useRef(serialize);

  const apply = useCallback(
    (change: Partial<C>, mode: HistoryMode) => {
      const {
        location: current,
        criteria: now,
        hasExplicitSort: explicit,
      } = latest.current;
      const write = (next: C, how: HistoryMode) => {
        const search = serializeRef.current(next).toString();
        const target = search === "" ? "" : `?${search}`;
        // Radix tabs may emit twice; pushing duplicates would make Back stay on this screen.
        if (how === "push" && target === (written.current ?? current.search)) return;
        written.current = target;
        navigate(
          { pathname: current.pathname, search: target },
          { replace: how === "replace" },
        );
      };
      // Materialize the current preferred sort before pushing so Back retains its order.
      if (mode === "push" && !explicit) write(now, "replace");
      write({ ...now, ...change }, mode);
    },
    [navigate],
  );

  return { criteria, apply };
}
