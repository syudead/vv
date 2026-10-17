import { useEffect } from "react";

/**
 * useEscapeClearsSelection は、何か選んでいる間（`active`）の Esc で選択を解除する。
 *
 * 選択バーのタグ操作の Combobox・ポップオーバーが Esc を自分の操作として使ったとき（候補の
 * 一覧や吹き出しを閉じる）は、preventDefault 済みなので見送り、選択を解除しない。選択バーから開いた窓（「Bundle as versions」）の Esc は窓を
 * 閉じるだけにし、選択を残す。窓が
 * 開いているかはフォーカスの位置によらずに見る。送る間は押したボタンが disabled になって
 * フォーカスが body に落ちるので、event.target で見ると選択を解除して窓ごと消し、送り終えた
 * 後の一覧の取り直しとトーストまで失ってしまうため。
 */
export function useEscapeClearsSelection(active: boolean, clear: () => void): void {
  useEffect(() => {
    if (!active) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      if (document.querySelector('[aria-modal="true"]') !== null) return;
      clear();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [active, clear]);
}
