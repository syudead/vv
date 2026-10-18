import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { subscribeServerEvents } from "../api/serverEvents";
import { subscribeSmartFolderChanges } from "../api/smartFolderChanges";
import { readSmartFolderCards } from "../api/smartFolders";
import { type CardValue, chunks, latestAnswer, withAnswer } from "./cardValueRules";

interface CardValues {
  values: ReadonlyMap<number, CardValue>;
  request: (id: number) => void;
}

const CardValuesContext = createContext<CardValues | null>(null);

const noValues: ReadonlyMap<number, CardValue> = new Map();

/**
 * Batch visible-card IDs in groups of 60; events and bulk-action changes refresh previously
 * requested IDs without skeletons, and reconnect refreshes all. Apply only each ID's latest request and retain values on
 * failure.
 */
export function SmartFolderCardsProvider({ children }: { children: ReactNode }) {
  const [values, setValues] = useState<ReadonlyMap<number, CardValue>>(noValues);
  const asked = useRef(new Set<number>());
  const queued = useRef<number[]>([]);
  const flushTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const controllers = useRef(new Set<AbortController>());
  /** Per-ID request generations prevent late responses from rolling values back. */
  const latestRead = useRef(new Map<number, number>());
  const readCount = useRef(0);

  const read = useCallback((ids: readonly number[]) => {
    for (const chunk of chunks(ids)) {
      const controller = new AbortController();
      controllers.current.add(controller);
      readCount.current += 1;
      const sent = readCount.current;
      for (const id of chunk) latestRead.current.set(id, sent);
      readSmartFolderCards(chunk, controller.signal).then(
        (cards) => {
          controllers.current.delete(controller);
          if (controller.signal.aborted) return;
          const answered = latestAnswer(
            chunk,
            cards,
            (id) => latestRead.current.get(id) === sent,
          );
          setValues((current) => withAnswer(current, answered.asked, answered.cards));
        },
        () => {
          controllers.current.delete(controller);
        },
      );
    }
  }, []);

  const request = useCallback(
    (id: number) => {
      if (asked.current.has(id)) return;
      asked.current.add(id);
      queued.current.push(id);
      if (flushTimer.current !== undefined) return;
      flushTimer.current = setTimeout(() => {
        flushTimer.current = undefined;
        const ids = queued.current;
        queued.current = [];
        read(ids);
      }, 0);
    },
    [read],
  );

  useEffect(() => {
    const readShown = (ids: readonly number[]) => {
      const shown = ids.filter((id) => asked.current.has(id));
      if (shown.length > 0) read(shown);
    };
    const unsubscribeChanges = subscribeSmartFolderChanges(readShown);
    const unsubscribe = subscribeServerEvents({
      smartFolder: readShown,
      open: (reconnected) => {
        if (reconnected && asked.current.size > 0) read(Array.from(asked.current));
      },
    });
    const live = controllers.current;
    return () => {
      unsubscribeChanges();
      unsubscribe();
      clearTimeout(flushTimer.current);
      for (const controller of live) controller.abort();
      live.clear();
    };
  }, [read]);

  const context = useMemo(() => ({ values, request }), [request, values]);
  return <CardValuesContext value={context}>{children}</CardValuesContext>;
}

/** Outside SmartFolderCardsProvider, never request data and always return undefined. */
export function useSmartFolderCardValue(id: number) {
  const context = useContext(CardValuesContext);
  const [node, setNode] = useState<Element | null>(null);
  const request = context?.request;
  useEffect(() => {
    if (node === null || request === undefined) return;
    if (typeof IntersectionObserver === "undefined") {
      request(id);
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      request(id);
      observer.disconnect();
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [id, node, request]);
  return { value: context?.values.get(id), ref: setNode };
}
