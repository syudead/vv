import { useLayoutEffect, useRef } from "react";

/**
 * useScrollTopOnChange は、一覧の条件（検索語・絞り込み・タグ・並べ替え）が変わったら
 * ページを先頭へ戻す。条件の違う一覧を、前の一覧のスクロール位置の途中から見せないため。
 * 最初の描画（再生から戻ったときの位置の復元を含む）では動かない。
 */
export function useScrollTopOnChange(signature: string) {
  const known = useRef(signature);
  useLayoutEffect(() => {
    if (known.current === signature) return;
    known.current = signature;
    window.scrollTo({ top: 0, behavior: "auto" });
  }, [signature]);
}
