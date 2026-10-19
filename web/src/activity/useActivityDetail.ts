import { useCallback, useEffect, useRef, useState } from "react";

import { type ActivityDetail, getActivity } from "../api/activity";
import { isAborted, RequestFailed } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { useWaiters } from "./useWaiters";

export type ActivityDetailState =
  | { kind: "loading" }
  | { kind: "notFound" }
  | { kind: "failed" }
  | { kind: "ready"; entry: ActivityDetail };

export interface ActivityDetailView {
  state: ActivityDetailState;
  retry: () => void;
  /** Resolve once the latest read settles, even when it fails. */
  refresh: () => Promise<void>;
}

/**
 * Read one entry and reread it on an activity notification or a reconnect, as the list does. A
 * failed reread keeps the shown entry; an invalid id reads as not found without a request.
 */
export function useActivityDetail(id: number | null): ActivityDetailView {
  const [state, setState] = useState<ActivityDetailState>(
    id === null ? { kind: "notFound" } : { kind: "loading" },
  );
  const stateRef = useRef(state);
  const show = useCallback((next: ActivityDetailState) => {
    stateRef.current = next;
    setState(next);
  }, []);
  const controller = useRef<AbortController | null>(null);
  const { wait, settle } = useWaiters();

  const read = useCallback(
    (initial: boolean) => {
      controller.current?.abort();
      if (id === null) {
        show({ kind: "notFound" });
        settle();
        return;
      }
      const current = new AbortController();
      controller.current = current;
      if (initial) show({ kind: "loading" });
      getActivity(id, current.signal).then(
        (entry) => {
          if (current.signal.aborted) return;
          show({ kind: "ready", entry });
          settle();
        },
        (error: unknown) => {
          if (isAborted(error) || current.signal.aborted) return;
          if (error instanceof RequestFailed && error.status === 404)
            show({ kind: "notFound" });
          else if (initial) show({ kind: "failed" });
          settle();
        },
      );
    },
    [id, settle, show],
  );

  // A reread replaces a pending one; every caller waits for the read that wins.
  const refresh = useCallback((): Promise<void> => {
    const kind = stateRef.current.kind;
    if (kind === "failed" || kind === "notFound") return Promise.resolve();
    const done = wait();
    read(kind === "loading");
    return done;
  }, [read, wait]);

  const retry = useCallback(() => read(true), [read]);

  useEffect(() => {
    read(true);
    const notified = () => void refresh();
    const unsubscribe = subscribeServerEvents({ activity: notified, open: notified });
    return () => {
      unsubscribe();
      controller.current?.abort();
      settle();
    };
  }, [read, refresh, settle]);

  return { state, retry, refresh };
}
