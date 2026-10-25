import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  type AssistantOutcome,
  type AssistantReviewRun,
  getAssistantReviewRun,
} from "../api/assistant";
import { subscribeServerEvents } from "../api/serverEvents";
import { isNewerRun, isRunning } from "./review";

export type ReviewRuns = ReadonlyMap<number, AssistantReviewRun>;

export interface ReviewRunsStore {
  /** The latest read of each review run a thread row shows, by run id. */
  reviews: ReviewRuns;
  /** A run the thread shows is still judging, so the trigger shows its dot while the panel is closed. */
  reviewing: boolean;
  settle: (outcome: AssistantOutcome) => void;
  clear: () => void;
}

function withRun(runs: ReviewRuns, run: AssistantReviewRun): ReviewRuns {
  const current = runs.get(run.id);
  if (current !== undefined && !isNewerRun(run, current)) return runs;
  return new Map(runs).set(run.id, run);
}

/** Follows the review runs started from the thread on assistant events until they finish. */
export function useReviewRuns(): ReviewRunsStore {
  const [reviews, setReviews] = useState<ReviewRuns>(new Map());
  const latest = useRef(reviews);
  latest.current = reviews;

  const settle = useCallback((outcome: AssistantOutcome) => {
    const run = outcome.run;
    if (outcome.kind !== "review" || run === undefined) return;
    setReviews((current) => withRun(current, run));
  }, []);

  const clear = useCallback(() => setReviews(new Map()), []);

  const reviewing = useMemo(() => [...reviews.values()].some(isRunning), [reviews]);

  useEffect(() => {
    if (!reviewing) return;
    const refresh = () => {
      for (const run of latest.current.values()) {
        if (!isRunning(run)) continue;
        // The progress row needs the run only; one candidate is the smallest page.
        getAssistantReviewRun(run.id, { limit: 1 }).then(
          (page) => setReviews((current) => withRun(current, page.run)),
          () => undefined,
        );
      }
    };
    return subscribeServerEvents({
      assistant: refresh,
      open: (reconnected) => {
        if (reconnected) refresh();
      },
    });
  }, [reviewing]);

  return { reviews, reviewing, settle, clear };
}
