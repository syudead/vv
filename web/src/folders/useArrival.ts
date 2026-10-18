import { useEffectEvent, useLayoutEffect, useRef } from "react";

/** Do not move focus on this tab's initial mount. */
let hasMounted = false;

/**
 * Focus the heading and scroll on folder navigation for accessibility; skip both when restoring a
 * snapshot.
 */
export function useArrival<T extends HTMLElement = HTMLHeadingElement>(
  restoring = false,
) {
  const heading = useRef<T | null>(null);
  const arrive = useEffectEvent(() => {
    if (!restoring) {
      window.scrollTo({ top: 0, behavior: "auto" });
      if (hasMounted) heading.current?.focus({ preventScroll: true });
    }
    hasMounted = true;
  });
  // Handle arrival once per keyed folder mount.
  useLayoutEffect(() => {
    arrive();
  }, []);
  return heading;
}
