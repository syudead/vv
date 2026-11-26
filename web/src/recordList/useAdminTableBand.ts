import {
  type RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

/** Measure sticky offsets for virtual scrolling and document scroll-padding-top so focused rows remain below headers. */
export function useAdminTableBand() {
  const bandRef = useRef<HTMLDivElement | null>(null);
  const [stuckBottom, setStuckBottom] = useState(0);
  const [headHeight, setHeadHeight] = useState(0);
  const [scrollMargin, setScrollMargin] = useState(0);

  const headRef = useCallback((head: HTMLTableSectionElement | null) => {
    if (head === null) return;
    const measure = () => setHeadHeight(head.offsetHeight);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(head);
    return () => {
      observer.disconnect();
      setHeadHeight(0);
    };
  }, []);
  const rowsTop = stuckBottom + headHeight;

  useLayoutEffect(() => {
    const band = bandRef.current;
    if (band === null) return;
    const measure = () => {
      const top = Number.parseFloat(getComputedStyle(band).top);
      setStuckBottom((Number.isFinite(top) ? top : 0) + band.offsetHeight);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(band);
    return () => observer.disconnect();
  }, []);

  // Restore document scroll padding on exit because the document outlives this page.
  useEffect(() => {
    const root = document.documentElement;
    const previous = root.style.scrollPaddingTop;
    root.style.scrollPaddingTop = `${String(rowsTop)}px`;
    return () => {
      root.style.scrollPaddingTop = previous;
    };
  }, [rowsTop]);

  function scrollListToTop(listBox: HTMLElement | null) {
    const listTop = listBox?.getBoundingClientRect().top;
    if (listTop !== undefined && listTop < stuckBottom) {
      window.scrollTo({ top: Math.max(0, window.scrollY + listTop - stuckBottom) });
    }
  }

  return {
    bandRef,
    headRef,
    stuckBottom,
    rowsTop,
    scrollMargin,
    setScrollMargin,
    scrollListToTop,
  };
}

/** Remeasure document offsets when creation rows or toolbar wrapping move the list; skip hidden lists. */
export function useListScrollMargin(
  listRef: RefObject<HTMLElement | null>,
  enabled: boolean,
  set: (margin: number) => void,
) {
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!enabled || list === null) return;
    const measure = () => set(list.getBoundingClientRect().top + window.scrollY);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(document.body);
    return () => observer.disconnect();
  }, [enabled, listRef, set]);
}
