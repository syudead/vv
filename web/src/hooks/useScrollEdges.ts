import { type RefObject, useEffect, useState } from "react";

export interface ScrollEdges {
  /** Content is hidden before the start edge. */
  start: boolean;
  /** Content is hidden past the end edge. */
  end: boolean;
}

const none: ScrollEdges = { start: false, end: false };

/** Subpixel layout can leave a fraction of a pixel of overflow that hides nothing. */
const tolerance = 1;

function scrollEdgesOf(element: HTMLElement): ScrollEdges {
  const hidden = element.scrollWidth - element.clientWidth;
  if (hidden <= tolerance) return none;
  return {
    start: element.scrollLeft > tolerance,
    end: element.scrollLeft < hidden - tolerance,
  };
}

/** Which sides of a horizontal scroller hide content, re-read on scroll and when it or its content resizes. */
export function useScrollEdges(ref: RefObject<HTMLElement | null>): ScrollEdges {
  const [edges, setEdges] = useState<ScrollEdges>(none);
  useEffect(() => {
    const element = ref.current;
    if (element === null) return;
    const update = () => {
      const next = scrollEdgesOf(element);
      setEdges((prev) =>
        prev.start === next.start && prev.end === next.end ? prev : next,
      );
    };
    update();
    element.addEventListener("scroll", update, { passive: true });
    const observer = new ResizeObserver(update);
    observer.observe(element);
    for (const child of element.children) observer.observe(child);
    return () => {
      element.removeEventListener("scroll", update);
      observer.disconnect();
    };
  }, [ref]);
  return edges;
}
