import { useCallback, useEffect, useRef, useState } from "react";

import { listTagPage, type TagPageQuery } from "../api/tags";

/** How many tags of the whole vocabulary are left to tidy; undefined while unread. */
export interface TagTallies {
  tentative?: number;
  noKind?: number;
  unused?: number;
}

const tallyQueries: Record<keyof TagTallies, TagPageQuery> = {
  tentative: { tentative: true },
  noKind: { kind: "none" },
  unused: { unused: true },
};

/** Each count is the total of a one-row page, so the screen API needs no count route. */
async function readTallies(signal: AbortSignal): Promise<TagTallies> {
  const entries = await Promise.all(
    Object.entries(tallyQueries).map(async ([key, query]) => {
      const page = await listTagPage({ ...query, limit: 1 }, signal);
      return [key, page.total] as const;
    }),
  );
  return Object.fromEntries(entries) as TagTallies;
}

/** The tidy-up counts, read whenever reload is called (with the list); a failed read keeps the last counts. */
export function useTagTallies(): { tallies: TagTallies; reload: () => void } {
  const [tallies, setTallies] = useState<TagTallies>({});
  const abortRef = useRef<AbortController | null>(null);

  const reload = useCallback(() => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    readTallies(controller.signal).then(
      (next) => {
        if (!controller.signal.aborted) setTallies(next);
      },
      () => undefined,
    );
  }, []);

  useEffect(() => () => abortRef.current?.abort(), []);

  return { tallies, reload };
}
