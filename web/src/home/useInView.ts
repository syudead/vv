import { useCallback, useEffect, useRef } from "react";

/**
 * useInView は要素が画面に入るたびに `onVisible` を呼ぶ ref を返す。画面の下の半分の高さだけ
 * 先に入ったと見なし、送る前に読み始める。IntersectionObserver の無い環境では、付けたときに
 * 1 回呼ぶ。
 */
export function useInView(onVisible: () => void) {
  const callback = useRef(onVisible);
  useEffect(() => {
    callback.current = onVisible;
  }, [onVisible]);
  const observer = useRef<IntersectionObserver | null>(null);
  useEffect(() => () => observer.current?.disconnect(), []);
  return useCallback((element: HTMLElement | null) => {
    observer.current?.disconnect();
    observer.current = null;
    if (element === null) return;
    if (typeof IntersectionObserver === "undefined") {
      callback.current();
      return;
    }
    const watcher = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) callback.current();
      },
      { rootMargin: "0px 0px 50% 0px" },
    );
    watcher.observe(element);
    observer.current = watcher;
  }, []);
}
