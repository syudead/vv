import { useCallback, useSyncExternalStore } from "react";
import { flushSync } from "react-dom";

import { readTheaterMode, writeTheaterMode } from "../preferences/theaterMode";

// Holds the choice only while storage refuses it, so tests that clear storage start from off.
let unstored: boolean | null = null;
const listeners = new Set<() => void>();

export function theaterMode(): boolean {
  return unstored ?? readTheaterMode();
}

export function setTheaterMode(on: boolean): void {
  unstored = writeTheaterMode(on) ? null : on;
  for (const listener of listeners) listener();
}

export function subscribeTheaterMode(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Theater mode is a per-device choice shared by every video page and the related-video list. */
export function useTheaterMode(): [boolean, () => void] {
  const on = useSyncExternalStore(subscribeTheaterMode, theaterMode);
  const toggle = useCallback(() => {
    const next = !theaterMode();
    // The transition snapshots the page, so the layout must be committed inside its callback.
    const apply = () => flushSync(() => setTheaterMode(next));
    if (!transitionsAllowed()) apply();
    else document.startViewTransition(apply);
  }, []);
  return [on, toggle];
}

function transitionsAllowed(): boolean {
  return (
    typeof document.startViewTransition === "function" &&
    !(
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    )
  );
}
