import { type RefObject, useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

/**
 * Attach to the section element with its ID; focus its h2 only on anchor navigation. Realign after
 * loading sections above it changes layout.
 */
export function useSectionAnchor(id: string): RefObject<HTMLDivElement | null> {
  const location = useLocation();
  const anchor = useRef<HTMLDivElement | null>(null);

  useLayoutEffect(() => {
    if (location.hash !== `#${id}`) return;
    const focusHeading = () => {
      const heading = anchor.current?.querySelector<HTMLHeadingElement>("h2");
      if (heading === null || heading === undefined) return;
      heading.tabIndex = -1;
      heading.scrollIntoView?.({ block: "start" });
      heading.focus();
    };
    focusHeading();
    const raf = window.requestAnimationFrame(focusHeading);
    const timers = [
      window.setTimeout(focusHeading, 0),
      window.setTimeout(focusHeading, 100),
    ];
    return () => {
      window.cancelAnimationFrame(raf);
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [id, location.hash, location.key]);

  return anchor;
}
