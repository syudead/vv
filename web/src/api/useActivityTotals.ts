import { useCallback, useEffect, useRef, useState } from "react";

import { type ActivityCounts, listActivity } from "./activity";
import { isAborted } from "./client";
import { subscribeServerEvents } from "./serverEvents";

/** Coalesce overlapping refreshes into one follow-up; disabled guests make no requests and initial failure remains null. */
export function useActivityTotals(enabled = true): ActivityCounts | null {
  const [counts, setCounts] = useState<ActivityCounts | null>(null);
  const fetching = useRef<AbortController | null>(null);
  const again = useRef(false);

  const load = useCallback(() => {
    if (fetching.current !== null) {
      again.current = true;
      return;
    }
    const controller = new AbortController();
    fetching.current = controller;
    again.current = false;
    const settle = () => {
      fetching.current = null;
      if (again.current) load();
    };
    listActivity({ limit: 1, signal: controller.signal }).then(
      (page) => {
        if (controller.signal.aborted) return;
        setCounts(page.counts);
        settle();
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        settle();
      },
    );
  }, []);

  useEffect(() => {
    if (!enabled) return;
    const unsubscribe = subscribeServerEvents({ activity: load, open: load });
    load();
    return () => {
      unsubscribe();
      fetching.current?.abort();
      fetching.current = null;
    };
  }, [enabled, load]);

  return enabled ? counts : null;
}
