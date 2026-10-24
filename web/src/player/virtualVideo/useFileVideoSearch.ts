import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted, listVideos, type Video } from "../../api/client";

const pageSize = 20;

type Status = "loading" | "ready" | "failed";

interface SearchState {
  rows: Video[];
  cursor: string | undefined;
  /** The first page. */
  status: Status;
  /** Pages after the first. */
  more: "idle" | Status;
}

export interface FileVideoSearch extends SearchState {
  hasMore: boolean;
  /** Re-read the page that failed. */
  retry: () => void;
  loadMore: () => void;
}

const firstPage: SearchState = {
  rows: [],
  cursor: undefined,
  status: "loading",
  more: "idle",
};

/** Pages through the owner's file videos matching query; a new query reads again from the first row. */
export function useFileVideoSearch(query: string): FileVideoSearch {
  const [state, setState] = useState<SearchState>(firstPage);
  const [generation, setGeneration] = useState(0);
  // One request at a time: a new first page aborts a pending next page.
  const pending = useRef<AbortController | null>(null);

  const read = useCallback(
    (cursor: string | undefined) => {
      pending.current?.abort();
      const controller = new AbortController();
      pending.current = controller;
      setState((current) =>
        cursor === undefined ? firstPage : { ...current, more: "loading" },
      );
      listVideos({
        query,
        kind: "file",
        cursor,
        limit: pageSize,
        signal: controller.signal,
      })
        .then((page) => {
          if (controller.signal.aborted) return;
          setState((current) => ({
            rows: cursor === undefined ? page.items : [...current.rows, ...page.items],
            cursor: page.nextCursor,
            status: "ready",
            more: "idle",
          }));
        })
        .catch((error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          setState((current) =>
            cursor === undefined
              ? { ...current, status: "failed" }
              : { ...current, more: "failed" },
          );
        });
    },
    [query],
  );

  useEffect(() => {
    read(undefined);
    return () => pending.current?.abort();
  }, [read, generation]);

  const { cursor, status, more } = state;
  const loadMore = useCallback(() => {
    if (status === "ready" && more === "idle" && cursor !== undefined) read(cursor);
  }, [cursor, more, read, status]);
  const retry = useCallback(() => {
    if (status === "failed") setGeneration((value) => value + 1);
    else if (more === "failed") read(cursor);
  }, [cursor, more, read, status]);

  return { ...state, hasMore: cursor !== undefined, retry, loadMore };
}
