import { useCallback, useState } from "react";

/** One viewport ahead, the margin at which a list asks for its next page. */
const nearMargin = "100% 0px";

type Observer = typeof IntersectionObserver;

/** Watches elements with one shared observer and calls each element's callback once, when it comes near. */
export function nearWatcher(Observer: Observer) {
  const callbacks = new Map<Element, () => void>();
  const observer = new Observer(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const callback = callbacks.get(entry.target);
        callbacks.delete(entry.target);
        observer.unobserve(entry.target);
        callback?.();
      }
    },
    { rootMargin: nearMargin },
  );
  return (element: Element, callback: () => void) => {
    callbacks.set(element, callback);
    observer.observe(element);
    return () => {
      callbacks.delete(element);
      observer.unobserve(element);
    };
  };
}

// Read once at load: a page's test that swaps in its own observer for load-more must not receive the images.
const NativeObserver = (globalThis as { IntersectionObserver?: Observer })
  .IntersectionObserver;
let watch: ReturnType<typeof nearWatcher> | null = null;

/**
 * Reports once whether the element came within a viewport of the screen and stays true after, so a list mounts
 * images it may never show without loading them. Without IntersectionObserver everything counts as near.
 */
export function useNearViewport<T extends Element>(): [
  (element: T | null) => (() => void) | undefined,
  boolean,
] {
  const [near, setNear] = useState(() => NativeObserver === undefined);
  const ref = useCallback(
    (element: T | null) => {
      if (element === null || near || NativeObserver === undefined) return undefined;
      watch ??= nearWatcher(NativeObserver);
      return watch(element, () => {
        setNear(true);
      });
    },
    [near],
  );
  return [ref, near];
}
