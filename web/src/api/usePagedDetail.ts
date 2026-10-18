import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "./client";
import type { ItemPage } from "./readThrough";

export type PagedDetailState<D> =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "gone" }
  | { kind: "ready"; data: D };

export type MorePageState = "idle" | "loading" | "failed";

function isGone(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

export interface PagedSource<D extends ItemPage<I>, I> {
  read: (id: number, atLeast: number, signal: AbortSignal) => Promise<D>;
  more: (id: number, cursor: string, signal: AbortSignal) => Promise<ItemPage<I>>;
  append: (current: readonly I[], page: readonly I[]) => I[];
  /** Capture a validity check before reading; discard the result if it later returns false. */
  beforeRead?: (quiet: boolean) => (data: D) => boolean;
  onMore?: (items: readonly I[]) => void;
}

/** Quiet refresh preserves content/count on failure; new reads supersede pending pages, and any 404 marks the entity gone. */
export function usePagedDetail<D extends ItemPage<I>, I>(
  id: number | null,
  source: PagedSource<D, I>,
) {
  const [state, setState] = useState<PagedDetailState<D>>(
    id === null ? { kind: "gone" } : { kind: "loading" },
  );
  const [more, setMore] = useState<MorePageState>("idle");
  const stateRef = useRef(state);
  const reading = useRef<AbortController | null>(null);
  const latest = useRef(source);
  useEffect(() => {
    latest.current = source;
  });

  const show = useCallback((next: PagedDetailState<D>) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const read = useCallback(
    (quiet: boolean) => {
      if (id === null) return;
      reading.current?.abort();
      const controller = new AbortController();
      reading.current = controller;
      const current = stateRef.current;
      const atLeast = current.kind === "ready" ? current.data.items.length : 0;
      if (!quiet) show({ kind: "loading" });
      setMore("idle");
      const accept = latest.current.beforeRead?.(quiet);
      latest.current.read(id, atLeast, controller.signal).then(
        (data) => {
          if (controller.signal.aborted) return;
          reading.current = null;
          if (accept === undefined || accept(data)) show({ kind: "ready", data });
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          reading.current = null;
          if (isGone(error)) show({ kind: "gone" });
          else if (!quiet || stateRef.current.kind !== "ready") show({ kind: "failed" });
        },
      );
    },
    [id, show],
  );

  const loadMore = useCallback(() => {
    const current = stateRef.current;
    if (id === null || current.kind !== "ready" || reading.current !== null) return;
    const cursor = current.data.nextCursor;
    if (cursor === undefined) return;
    const controller = new AbortController();
    reading.current = controller;
    setMore("loading");
    latest.current.more(id, cursor, controller.signal).then(
      (page) => {
        if (controller.signal.aborted) return;
        reading.current = null;
        const now = stateRef.current;
        if (now.kind !== "ready") return;
        setMore("idle");
        latest.current.onMore?.(page.items);
        show({
          kind: "ready",
          data: {
            ...now.data,
            items: latest.current.append(now.data.items, page.items),
            nextCursor: page.nextCursor,
          },
        });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        reading.current = null;
        setMore(isGone(error) ? "idle" : "failed");
        if (isGone(error)) show({ kind: "gone" });
      },
    );
  }, [id, show]);

  const update = useCallback(
    (change: (data: D) => D) => {
      const current = stateRef.current;
      if (current.kind !== "ready") return;
      show({ kind: "ready", data: change(current.data) });
    },
    [show],
  );

  const markGone = useCallback(() => {
    reading.current?.abort();
    reading.current = null;
    setMore("idle");
    show({ kind: "gone" });
  }, [show]);

  const stop = useCallback(() => {
    reading.current?.abort();
    reading.current = null;
  }, []);

  return { state, stateRef, more, read, loadMore, update, markGone, stop };
}
