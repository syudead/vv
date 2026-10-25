import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  type AssistantProposal,
  type AssistantReviewCandidate,
  type AssistantReviewRun,
  getAssistantProposal,
  getAssistantReviewRun,
} from "../api/assistant";
import { RequestFailed } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { isActive, isNewer } from "./proposal";
import { isNewerRun, shownCandidates, stateCounts, type StateCounts } from "./review";

/** Rows per read, as the tags admin pages them. */
export const reviewPageSize = 100;
/** The largest page the server answers; a refresh re-reads the loaded rows in pages of it. */
const refreshPageSize = 200;

export type More = "idle" | "loading" | "failed";

export type ReviewLoad =
  | { kind: "loading" }
  | { kind: "notFound" }
  | { kind: "failed"; error: unknown }
  | {
      kind: "ready";
      run: AssistantReviewRun;
      /** Every candidate read, rejected ones included, most probable first. */
      read: AssistantReviewCandidate[];
      nextCursor?: string;
      more: More;
    };

export interface ReviewPage {
  load: ReviewLoad;
  rows: AssistantReviewCandidate[];
  counts: StateCounts;
  /** The proposals adopted on this page, by candidate id; a reload forgets them. */
  proposals: ReadonlyMap<number, AssistantProposal>;
  retry: () => void;
  loadMore: () => void;
  refresh: () => void;
  markAdopted: (adopted: ReadonlyMap<number, AssistantProposal>) => void;
  markRejected: (ids: readonly number[]) => void;
}

function isNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/** Reads from the first candidate until at least `count` are read again, or the run has no more. */
async function readFromStart(id: number, count: number) {
  let page = await getAssistantReviewRun(id, {
    limit: Math.max(reviewPageSize, Math.min(count, refreshPageSize)),
  });
  const read = [...page.items];
  while (read.length < count && page.nextCursor !== undefined) {
    page = await getAssistantReviewRun(id, {
      cursor: page.nextCursor,
      limit: refreshPageSize,
    });
    read.push(...page.items);
  }
  return { run: page.run, read, nextCursor: page.nextCursor };
}

function withState(
  candidates: readonly AssistantReviewCandidate[],
  ids: ReadonlySet<number>,
  state: AssistantReviewCandidate["state"],
): AssistantReviewCandidate[] {
  return candidates.map((candidate) =>
    ids.has(candidate.id) ? { ...candidate, state } : candidate,
  );
}

/** Keeps the latest of two reads of one proposal, as the thread's cards do. */
function withProposal(
  proposals: ReadonlyMap<number, AssistantProposal>,
  proposal: AssistantProposal,
): ReadonlyMap<number, AssistantProposal> {
  let next: Map<number, AssistantProposal> | undefined;
  for (const [candidateId, current] of proposals) {
    if (current.id !== proposal.id || !isNewer(proposal, current)) continue;
    next ??= new Map(proposals);
    next.set(candidateId, proposal);
  }
  return next ?? proposals;
}

/**
 * The review list's run and candidates. Every assistant event re-reads the loaded rows and the
 * proposals adopted here, one read at a time.
 */
export function useReviewPage(id: number): ReviewPage {
  const [load, setLoad] = useState<ReviewLoad>({ kind: "loading" });
  const latest = useRef(load);
  latest.current = load;
  const [kept, setKept] = useState<ReadonlyMap<number, AssistantReviewCandidate>>(
    new Map(),
  );
  const [proposals, setProposals] = useState<ReadonlyMap<number, AssistantProposal>>(
    new Map(),
  );
  const latestProposals = useRef(proposals);
  latestProposals.current = proposals;
  const reading = useRef(false);
  const dirty = useRef(false);
  const alive = useRef(true);

  const refresh = useCallback(() => {
    if (reading.current) {
      dirty.current = true;
      return;
    }
    reading.current = true;
    dirty.current = false;
    const current = latest.current;
    const count = current.kind === "ready" ? current.read.length : 0;
    readFromStart(id, count)
      .then(
        ({ run, read, nextCursor }) => {
          if (!alive.current) return;
          setLoad((previous) => {
            const more = previous.kind === "ready" ? previous.more : "idle";
            const shown =
              previous.kind === "ready" && !isNewerRun(run, previous.run)
                ? previous.run
                : run;
            return { kind: "ready", run: shown, read, nextCursor, more };
          });
        },
        (error: unknown) => {
          if (!alive.current) return;
          setLoad((previous) => {
            if (isNotFound(error)) return { kind: "notFound" };
            // A failed re-read keeps the rows shown; only the first read shows the error.
            return previous.kind === "ready" ? previous : { kind: "failed", error };
          });
        },
      )
      .finally(() => {
        reading.current = false;
        if (dirty.current && alive.current) refresh();
      });
  }, [id]);

  const refreshProposals = useCallback(() => {
    const ids = new Set<number>();
    for (const proposal of latestProposals.current.values()) {
      if (isActive(proposal.status)) ids.add(proposal.id);
    }
    for (const proposalId of ids) {
      getAssistantProposal(proposalId).then(
        (proposal) => {
          if (alive.current) setProposals((current) => withProposal(current, proposal));
        },
        () => undefined,
      );
    }
  }, []);

  const retry = useCallback(() => {
    setLoad({ kind: "loading" });
    refresh();
  }, [refresh]);

  useEffect(() => {
    alive.current = true;
    refresh();
    const onEvent = () => {
      refresh();
      refreshProposals();
    };
    const unsubscribe = subscribeServerEvents({
      assistant: onEvent,
      open: (reconnected) => {
        if (reconnected) onEvent();
      },
    });
    return () => {
      alive.current = false;
      unsubscribe();
    };
  }, [refresh, refreshProposals]);

  const loadMore = useCallback(() => {
    const current = latest.current;
    if (current.kind !== "ready" || current.nextCursor === undefined) return;
    if (current.more === "loading") return;
    const cursor = current.nextCursor;
    setLoad({ ...current, more: "loading" });
    getAssistantReviewRun(id, { cursor, limit: reviewPageSize }).then(
      (page) => {
        if (!alive.current) return;
        setLoad((previous) => {
          if (previous.kind !== "ready" || previous.nextCursor !== cursor)
            return previous;
          const known = new Set(previous.read.map((candidate) => candidate.id));
          return {
            ...previous,
            read: [...previous.read, ...page.items.filter((item) => !known.has(item.id))],
            nextCursor: page.nextCursor,
            more: "idle",
          };
        });
      },
      () => {
        if (!alive.current) return;
        setLoad((previous) =>
          previous.kind === "ready" ? { ...previous, more: "failed" } : previous,
        );
      },
    );
  }, [id]);

  const markAdopted = useCallback((adopted: ReadonlyMap<number, AssistantProposal>) => {
    const ids = new Set(adopted.keys());
    const current = latest.current;
    if (current.kind === "ready") {
      setKept((previous) => {
        const next = new Map(previous);
        for (const candidate of current.read) {
          if (ids.has(candidate.id))
            next.set(candidate.id, { ...candidate, state: "adopted" });
        }
        return next;
      });
    }
    setProposals((previous) => new Map([...previous, ...adopted]));
    setLoad((previous) =>
      previous.kind === "ready"
        ? { ...previous, read: withState(previous.read, ids, "adopted") }
        : previous,
    );
  }, []);

  const markRejected = useCallback((ids: readonly number[]) => {
    const rejected = new Set(ids);
    setLoad((previous) =>
      previous.kind === "ready"
        ? { ...previous, read: withState(previous.read, rejected, "rejected") }
        : previous,
    );
  }, []);

  const read = load.kind === "ready" ? load.read : noCandidates;
  const rows = useMemo(() => shownCandidates(read, kept), [read, kept]);
  const counts = useMemo(() => stateCounts(read, kept), [read, kept]);

  return {
    load,
    rows,
    counts,
    proposals,
    retry,
    loadMore,
    refresh,
    markAdopted,
    markRejected,
  };
}

const noCandidates: AssistantReviewCandidate[] = [];
