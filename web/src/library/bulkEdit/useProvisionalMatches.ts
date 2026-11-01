import { useEffect, useRef, useState } from "react";

import {
  matchProvisionalPeople,
  type ProvisionalMatch,
  type VideoEditTargets,
} from "../../api/videoEdits";

/** Matches for the current targets and added people; undefined while reading, empty after a failure. */
export function useProvisionalMatches(
  targets: VideoEditTargets,
  targetKey: string,
  personIds: readonly number[],
  enabled: boolean,
): ProvisionalMatch[] | undefined {
  const [matches, setMatches] = useState<ProvisionalMatch[] | undefined>(undefined);
  // The keys identify the inputs; the arrays are rebuilt on every render.
  const latest = useRef({ targets, personIds });
  latest.current = { targets, personIds };
  const personKey = personIds.join(",");
  useEffect(() => {
    if (!enabled) return;
    const { targets, personIds } = latest.current;
    const controller = new AbortController();
    setMatches(undefined);
    matchProvisionalPeople(targets, personIds, controller.signal).then(
      (items) => {
        if (!controller.signal.aborted) setMatches(items);
      },
      () => {
        // Without matches nothing is suggested; the owner can still choose every merge.
        if (!controller.signal.aborted) setMatches([]);
      },
    );
    return () => controller.abort();
  }, [targetKey, personKey, enabled]);
  return enabled ? matches : undefined;
}
