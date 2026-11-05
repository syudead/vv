import { useEffect, useRef, useState } from "react";

import {
  matchProvisionalPeople,
  type ProvisionalMatch,
  type VideoEditSummary,
  type VideoEditTargets,
} from "../../api/videoEdits";
import type { BulkDraft } from "./draft";
import {
  effectiveMerges,
  type MergeCandidate,
  mergeCandidates,
  type MergeTarget,
  mergeTargets,
  suggestMerges,
  targetIds,
} from "./merges";

export interface MergeState {
  targets: MergeTarget[];
  candidates: MergeCandidate[];
  /** What Apply merges, by candidate ID to target key. */
  merges: Map<number, string>;
  /** True when the field shows: the draft adds a performer and the selection has unnamed ones. */
  shown: boolean;
  /** True while the suggestions are read; Apply waits so it sends the merges on screen. */
  loading: boolean;
}

/** The merge field's state for the draft and the current targets. */
export function useMergeState(
  draft: BulkDraft,
  summary: VideoEditSummary | undefined,
  targets: VideoEditTargets,
  targetKey: string,
): MergeState {
  const mergeTo = summary === undefined ? [] : mergeTargets(draft, summary);
  const candidates = summary === undefined ? [] : mergeCandidates(draft, summary);
  const shown = mergeTo.length > 0 && candidates.length > 0;
  const matches = useProvisionalMatches(targets, targetKey, targetIds(mergeTo), shown);
  const merges =
    matches === undefined
      ? new Map<number, string>()
      : effectiveMerges(
          draft,
          candidates,
          suggestMerges(candidates, matches, mergeTo),
          mergeTo,
        );
  return {
    targets: mergeTo,
    candidates,
    merges,
    shown,
    loading: shown && matches === undefined,
  };
}

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
