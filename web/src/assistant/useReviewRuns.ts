import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { type AssistantReviewRun, getAssistantReviewRun } from "../api/assistant";
import { subscribeServerEvents } from "../api/serverEvents";
import { isNewerRun, isRunning } from "./review";

export type ReviewRuns = ReadonlyMap<number, AssistantReviewRun>;

export interface ReviewRunsStore {
  /** The latest read of each review run a thread row shows, by run id. */
  reviews: ReviewRuns;
  /** A run the thread shows is still judging, so the trigger shows its dot while the panel is closed. */
  reviewing: boolean;
}

function withRun(runs: ReviewRuns, run: AssistantReviewRun): ReviewRuns {
  const current = runs.get(run.id);
  if (current !== undefined && !isNewerRun(run, current)) return runs;
  return new Map(runs).set(run.id, run);
}

/** Reads the review runs the thread's turns started, and follows them on assistant events until they finish. */
export function useReviewRuns(ids: readonly number[]): ReviewRunsStore {
  const [reviews, setReviews] = useState<ReviewRuns>(new Map());
  const latest = useRef(reviews);
  latest.current = reviews;
  const idsKey = ids.join(",");

  // The progress row needs the run only; one candidate is the smallest page.
  const read = useCallback((id: number) => {
    getAssistantReviewRun(id, { limit: 1 }).then(
      (page) => setReviews((current) => withRun(current, page.run)),
      () => undefined,
    );
  }, []);

  useEffect(() => {
    for (const id of idsKey === "" ? [] : idsKey.split(",").map(Number)) {
      if (!latest.current.has(id)) read(id);
    }
  }, [idsKey, read]);

  // A cleared thread no longer names its runs, so they stop counting as working.
  const runningIds = useMemo(() => {
    const named = new Set(idsKey.split(","));
    return [...reviews.values()]
      .filter((run) => isRunning(run) && named.has(String(run.id)))
      .map((run) => run.id)
      .join(",");
  }, [reviews, idsKey]);
  const reviewing = runningIds !== "";

  useEffect(() => {
    if (runningIds === "") return;
    const refresh = () => {
      for (const id of runningIds.split(",")) read(Number(id));
    };
    const unsubscribe = subscribeServerEvents({
      assistant: refresh,
      open: (reconnected) => {
        if (reconnected) refresh();
      },
    });
    // A run that finished after its first read and before this subscription sends no later event.
    refresh();
    return unsubscribe;
  }, [runningIds, read]);

  return { reviews, reviewing };
}
