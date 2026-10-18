import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { listWatchHistoryDates, type WatchHistoryFilter } from "../api/history";

export type HistoryDatesState =
  { kind: "loading" } | { kind: "failed" } | { kind: "ready"; days: readonly string[] };

export interface HistoryDates {
  state: HistoryDatesState;
  retry: () => void;
  /** Clear after deleting all history and discard in-flight responses. */
  clear: () => void;
}

/** Refetch on watch or query changes, but not date selection, to avoid flickering navigation. */
export function useHistoryDates(watch: WatchHistoryFilter, query: string): HistoryDates {
  const [state, setState] = useState<HistoryDatesState>({ kind: "loading" });
  const controller = useRef<AbortController | null>(null);

  const stop = useCallback(() => {
    controller.current?.abort();
    controller.current = null;
  }, []);

  const read = useCallback(() => {
    stop();
    const current = new AbortController();
    controller.current = current;
    setState({ kind: "loading" });
    listWatchHistoryDates({
      ...(watch === "all" ? {} : { watch }),
      ...(query === "" ? {} : { query }),
      signal: current.signal,
    }).then(
      (dates) => {
        if (controller.current !== current) return;
        controller.current = null;
        setState({ kind: "ready", days: dates.days });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.current !== current) return;
        controller.current = null;
        setState({ kind: "failed" });
      },
    );
  }, [query, stop, watch]);

  useEffect(() => {
    read();
    return stop;
  }, [read, stop]);

  const clear = useCallback(() => {
    stop();
    setState({ kind: "ready", days: [] });
  }, [stop]);

  return { state, retry: read, clear };
}
