// 画面の文字をクリップボードへ写す。再生画面の「パスをコピー」と設定画面の「Copy token」が
// 共有する（specs/026-external-api/ui-design.md「Reveal」）。

/**
 * copyWithSelection は見えない入力欄に文字を置いて選び、コピーの命令で写す。
 * 選ぶと入力欄にフォーカスが移るので、終わったら元の要素（「パスをコピー」など、押したボタン）へ戻す。
 */
function copyWithSelection(text: string): boolean {
  const previous = document.activeElement;
  const field = document.createElement("textarea");
  field.value = text;
  field.setAttribute("readonly", "");
  field.style.position = "fixed";
  field.style.opacity = "0";
  document.body.appendChild(field);
  field.select();
  try {
    // 代わりの無い古い API だが、安全でない接続ではこれしか使えない。
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    field.remove();
    if (previous instanceof HTMLElement && previous.isConnected) previous.focus();
  }
}

/**
 * copyText は text をクリップボードへ写し、結果を onCopied か onFailed で知らせる。
 * navigator.clipboard は安全な接続（HTTPS・localhost）でしか使えない。LAN のアドレスで
 * 開いたときは、選んだ文字をコピーする古い方法に切り替える（その場合は同期で知らせる）。
 */
export function copyText(
  text: string,
  { onCopied, onFailed }: { onCopied: () => void; onFailed: () => void },
): void {
  const fallback = () => (copyWithSelection(text) ? onCopied() : onFailed());
  if (navigator.clipboard === undefined) {
    fallback();
    return;
  }
  void navigator.clipboard.writeText(text).then(onCopied, fallback);
}
