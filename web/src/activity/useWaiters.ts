import { useCallback, useMemo, useRef } from "react";

export interface Waiters {
  /** A promise that resolves at the next settle. */
  wait: () => Promise<void>;
  settle: () => void;
}

/** Callers waiting for every pending reread to settle, resolved together. */
export function useWaiters(): Waiters {
  const waiters = useRef<(() => void)[]>([]);
  const wait = useCallback(
    () =>
      new Promise<void>((resolve) => {
        waiters.current.push(resolve);
      }),
    [],
  );
  const settle = useCallback(() => {
    const waiting = waiters.current;
    waiters.current = [];
    for (const resolve of waiting) resolve();
  }, []);
  return useMemo(() => ({ wait, settle }), [wait, settle]);
}
