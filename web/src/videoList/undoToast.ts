import { useCallback, useEffect, useRef } from "react";

import { RequestFailed } from "../api/client";
import { undoOrganize } from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";

// ドラッグの操作の取り消しの通知である（specs/050-drag-organize/ui-design.md「Undo toast」、
// research.md R-9）。受け入れられた操作ごとに「Undo」付きの通知を 8 秒出し、次の操作の通知が
// 前のものを置き換える（ToastProvider がアクション付きの通知を 1 件に保つ）。

/** undoFailedText は取り消しの失敗の文言である。 */
export function undoFailedText(error: unknown): UiText {
  if (error instanceof RequestFailed) {
    if (error.status === 409) return t.organize.undoFailed.conflict;
    if (error.status === 410) return t.organize.undoFailed.expired;
  }
  return t.organize.undoFailed.other(errorText(error));
}

/** UndoToast は画面が使う取り消しの通知である（useUndoToast の戻り値）。 */
export interface UndoToast {
  /** 受け入れられた操作の通知 `message` を、トークン `token` の「Undo」付きで出す。 */
  offer: (message: UiText, token: string) => void;
  /**
   * carry は今の通知とトークンを、この画面を離れても残す。操作の結果として画面が移るとき
   * （開いているグループが解散した）に、移る前に呼ぶ（ui-design.md「Undo toast」の Carried）。
   */
  carry: () => void;
}

/**
 * useUndoToast は取り消しの通知を出す。「Undo」を押すと POST /api/organize/undo を送り、
 * 成功なら「Undone」、失敗なら理由を出し、どちらも `reload` で一覧を読み直す。画面を離れると
 * 通知を閉じてトークンを忘れる（carry した通知は残す）。
 */
export function useUndoToast(reload: () => void): UndoToast {
  const toast = useToast();
  const reloadRef = useRef(reload);
  useEffect(() => {
    reloadRef.current = reload;
  }, [reload]);
  const dismiss = useRef<(() => void) | null>(null);
  const carried = useRef(false);

  useEffect(
    () => () => {
      if (!carried.current) dismiss.current?.();
      dismiss.current = null;
    },
    [],
  );

  const offer = useCallback(
    (message: UiText, token: string) => {
      carried.current = false;
      dismiss.current = toast(message, {
        action: {
          label: t.organize.undo,
          run: async () => {
            try {
              await undoOrganize(token);
              toast(t.organize.undone);
            } catch (error) {
              toast(undoFailedText(error));
            }
            dismiss.current = null;
            reloadRef.current();
          },
        },
      });
    },
    [toast],
  );

  const carry = useCallback(() => {
    carried.current = true;
  }, []);

  return { offer, carry };
}
