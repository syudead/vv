import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

import { apiWriteCount } from "../../api/client";
import type { PeoplePageRows, PeopleRowsQuery } from "./peopleCriteria";

export interface PeopleSnapshot {
  page: PeoplePageRows;
  search: string;
  scrollY: number;
}

/** One snapshot, so Back from a person page returns to the same rows and scroll position. */
let held: (PeopleSnapshot & { key: string; writes: number }) | undefined;

export function clearPeopleSnapshot(): void {
  held = undefined;
}

export function peopleSnapshotKey(query: PeopleRowsQuery): string {
  return JSON.stringify([query.query, query.sort, query.favorite, query.named]);
}

export function savePeopleSnapshot(snapshot: PeopleSnapshot): void {
  held = {
    ...snapshot,
    key: peopleSnapshotKey(snapshot.page.query),
    writes: apiWriteCount(),
  };
}

/** Any write since the save may have renamed, merged or removed a person, so the grid loads afresh. */
export function takePeopleSnapshot(key: string): PeopleSnapshot | undefined {
  if (held === undefined || held.key !== key || held.writes !== apiWriteCount()) {
    return undefined;
  }
  return held;
}

/**
 * Restores the snapshot for `key` once on mount and scrolls back to it after its rows render.
 * `skipLoad` answers true while the conditions are still the restored ones.
 */
export function usePeopleSnapshot(key: string) {
  const [restored] = useState(() => takePeopleSnapshot(key));
  const restoredKey = useRef(restored === undefined ? undefined : key);
  const pendingScroll = useRef(restored?.scrollY);

  useEffect(() => {
    const previous = history.scrollRestoration;
    history.scrollRestoration = "manual";
    return () => {
      history.scrollRestoration = previous;
    };
  }, []);
  useLayoutEffect(() => {
    const top = pendingScroll.current;
    if (top === undefined) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    // The top bar portal and face images may shift the layout a frame later.
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, []);

  const skipLoad = useCallback((current: string) => {
    if (restoredKey.current === current) return true;
    restoredKey.current = undefined;
    return false;
  }, []);

  return { restored, skipLoad };
}
