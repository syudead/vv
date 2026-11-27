import { useCallback, useEffect, useRef, useState } from "react";

import { getTagTallies, type TagTallyCounts } from "../api/tags";

/** How many tags of the whole vocabulary are left to tidy; undefined while unread. */
export type TagTallies = Partial<TagTallyCounts>;

/** The tidy-up counts, read whenever reload is called (with the list); a failed read keeps the last counts. */
export function useTagTallies(): { tallies: TagTallies; reload: () => void } {
  const [tallies, setTallies] = useState<TagTallies>({});
  const abortRef = useRef<AbortController | null>(null);

  const reload = useCallback(() => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    getTagTallies(controller.signal).then(
      (next) => {
        if (!controller.signal.aborted) setTallies(next);
      },
      () => undefined,
    );
  }, []);

  useEffect(() => () => abortRef.current?.abort(), []);

  return { tallies, reload };
}
