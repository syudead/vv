import { useLayoutEffect, useRef } from "react";

/**
 * useScrollTopOnChange は、表示の条件（一覧の検索語・絞り込み・タグ・並べ替え、設定のページ）が
 * 変わったらページを先頭へ戻す。条件の違う内容を、前の内容のスクロール位置の途中から見せないため。
 * 最初の描画（再生から戻ったときの位置の復元を含む）では動かない。`undefined` は条件がまだ
 * 決まっていないこと（アドレスの置き換えの途中など）を表して何もせず、その後に初めて決まった条件を
 * 最初の描画として扱う。
 */
export function useScrollTopOnChange(signature: string | undefined) {
  const known = useRef(signature);
  useLayoutEffect(() => {
    if (signature === undefined || known.current === signature) return;
    const first = known.current === undefined;
    known.current = signature;
    if (!first) window.scrollTo({ top: 0, behavior: "auto" });
  }, [signature]);
}
