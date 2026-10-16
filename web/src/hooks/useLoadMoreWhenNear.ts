import { type RefObject, useEffect } from "react";

/**
 * useLoadMoreWhenNear は、`target` の要素が画面の下から 1 画面分の内に来たら `loadMore` を
 * 呼ぶ。`enabled` が false の間（読んでいる間、続きが無いとき）は見ない。`version` が変わる
 * たび（行が増えたとき）に見直す。視聴履歴とアクティビティの一覧が使う。
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
