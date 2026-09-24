import { useLayoutEffect, useRef } from "react";

/** hasMounted はこのタブで最初の表示が済んだかを覚える。最初の表示ではフォーカスを動かさない。 */
let hasMounted = false;

/**
 * useArrival は別のフォルダへ移ったとき、見出しへフォーカスを移して先頭へ
 * スクロールする。読み上げソフトの利用者が移動と現在地を知れるようにする。
 * 控えから戻ってきたとき（restoring）は、元の位置へ戻すのでどちらもしない。
 */
export function useArrival(restoring = false) {
  const heading = useRef<HTMLHeadingElement | null>(null);
  useLayoutEffect(() => {
    if (!restoring) {
      window.scrollTo({ top: 0, behavior: "auto" });
      if (hasMounted) heading.current?.focus({ preventScroll: true });
    }
    hasMounted = true;
    // 到着はフォルダごとに1回だけ扱う（FolderView は key でフォルダごとに作り直す）。
  }, []);
  return heading;
}
