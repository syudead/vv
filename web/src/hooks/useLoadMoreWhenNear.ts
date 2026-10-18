import { type RefObject, useEffect } from "react";

/**
 * Prefetch within one viewport of target; version reattaches observation after row changes, while
 * enabled gates loading.
 */
export function useLoadMoreWhenNear(
  target: RefObject<Element | null>,
  enabled: boolean,
  loadMore: () => void,
  version: unknown,
): void {
  useEffect(() => {
    const element = target.current;
    if (!enabled || element === null || typeof IntersectionObserver === "undefined")
      return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "0px 0px 100% 0px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [target, enabled, loadMore, version]);
}
