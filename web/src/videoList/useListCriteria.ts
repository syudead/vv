import { useCallback, useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router";

import type { VideoSort } from "../api/client";
import { useAudience } from "../auth/audience";
import {
  guestListCriteria,
  type HistoryMode,
  type ListCriteria,
  type ListCriteriaChange,
  newSeed,
  ownerOnlySort,
  parseListCriteria,
  serializeListCriteria,
} from "./listCriteria";

export type ExtraValues = Readonly<Record<string, readonly string[]>>;

export interface ListCriteriaState {
  criteria: ListCriteria;
  /**
   * Preserve unmentioned extra parameters, including values written by an earlier apply before the
   * next render.
   */
  apply: (
    next: ListCriteriaChange,
    mode: HistoryMode,
    extra?: ExtraValues,
    nativeOrder?: boolean,
  ) => void;
  nativeOrder: boolean;
}

const noExtraParams: readonly string[] = [];

/**
 * Preserve declared extraParams through every URL rewrite; supply stable extraParams and
 * guestParams arrays. Guests normalize owner-only filters and sorts in the URL without modifying
 * stored preferences.
 */
export function useListCriteria(
  preferredSort: VideoSort,
  extraParams: readonly string[] = noExtraParams,
  guestParams: readonly string[] = noExtraParams,
  nativeDefault = false,
): ListCriteriaState {
  const location = useLocation();
  const navigate = useNavigate();
  const guest = useAudience() === "guest";
  const searchParams = new URLSearchParams(location.search);
  const parsed = parseListCriteria(searchParams, preferredSort);
  const rawExtra: ExtraValues = Object.fromEntries(
    extraParams.map((name) => [name, searchParams.getAll(name)]),
  );
  const ownerOnlyExtra = Object.entries(rawExtra).filter(
    ([name]) => !guestParams.includes(name),
  );
  const guestLeftovers =
    guest &&
    (parsed.criteria.favorite ||
      (parsed.hasExplicitSort && ownerOnlySort(parsed.criteria.sort)) ||
      ownerOnlyExtra.some(([, values]) => values.length > 0));
  const extra: ExtraValues = guest
    ? Object.fromEntries(
        Object.entries(rawExtra).filter(([name]) => guestParams.includes(name)),
      )
    : rawExtra;

  // Generate one seed per URL and replace history; use the same seed before the replacement renders.
  const supplied = useRef<{ search: string; seed: number } | null>(null);
  let criteria = guest ? guestListCriteria(parsed.criteria) : parsed.criteria;
  if (parsed.needsSeed) {
    if (supplied.current?.search !== location.search) {
      supplied.current = { search: location.search, seed: newSeed() };
    }
    criteria = { ...criteria, seed: supplied.current.seed };
  }

  const latest = useRef({
    location,
    criteria,
    hasExplicitSort: parsed.hasExplicitSort,
    extra,
    nativeOrder: nativeDefault && !parsed.hasExplicitSort,
  });
  latest.current = {
    location,
    criteria,
    hasExplicitSort: parsed.hasExplicitSort,
    extra,
    nativeOrder: nativeDefault && !parsed.hasExplicitSort,
  };

  const write = useCallback(
    (
      next: ListCriteria,
      mode: HistoryMode,
      extraValues?: ExtraValues,
      nativeOrder = latest.current.nativeOrder,
    ) => {
      const { location: current } = latest.current;
      const params = serializeListCriteria(next);
      if (nativeOrder) params.delete("sort");
      const written: Record<string, readonly string[]> = {};
      for (const name of extraParams) {
        const values = extraValues?.[name] ?? latest.current.extra[name] ?? [];
        written[name] = values;
        for (const value of values) params.append(name, value);
      }
      // Remember writes immediately so multiple applies before rendering compose instead of overwriting each other.
      latest.current = {
        ...latest.current,
        criteria: next,
        extra: written,
        nativeOrder,
        hasExplicitSort: params.has("sort"),
      };
      const search = params.toString();
      navigate(
        { pathname: current.pathname, search: search === "" ? "" : `?${search}` },
        { replace: mode === "replace" },
      );
    },
    [navigate, extraParams],
  );

  const needsSeed = parsed.needsSeed;
  useEffect(() => {
    if (needsSeed || guestLeftovers) write(latest.current.criteria, "replace");
  }, [needsSeed, guestLeftovers, location.search, write]);

  const apply = useCallback(
    (
      change: ListCriteriaChange,
      mode: HistoryMode,
      extraValues?: ExtraValues,
      nativeOrder = latest.current.nativeOrder,
    ) => {
      const { criteria: current, hasExplicitSort } = latest.current;
      const next = typeof change === "function" ? change(current) : change;
      // Before pushing history, materialize the current sort in an implicit-sort URL so Back preserves the prior order after stored preferences change; retain extras.
      if (mode === "push" && !hasExplicitSort && !nativeDefault)
        write(current, "replace");
      write(next, mode, extraValues, nativeOrder);
    },
    [write, nativeDefault],
  );

  return { criteria, apply, nativeOrder: nativeDefault && !parsed.hasExplicitSort };
}
