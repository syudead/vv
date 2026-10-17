import { useCallback } from "react";

import { type ActivityEntry, retryActivity, undoActivity } from "../api/activity";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";

// アクティビティのカードのやり直しと取り消し（specs/055-activity/ui-design.md「Cards」）。

export interface ActivityOperations {
  retry: (entry: ActivityEntry) => Promise<void>;
  undo: (entry: ActivityEntry, tagId?: number) => Promise<void>;
}

/**
 * useActivityOperations はカードのやり直しと取り消しを送る口を返す。どちらも確かめずに送り、
 * 成功したら一覧を読み直す（サーバーの `activity` の知らせより先に見せる）。取り消しの後は
 * 結果に応じた通知を、失敗ではカードをそのままに理由の通知を出す。返す Promise は拒否せず、
 * 成功したときは読み直しが終わってから解決する。それまでカードは古い `retryRun`・`undoable`
 * のままなので、カードはボタンを止めたままにして同じ操作を二度送らせない。
 */
export function useActivityOperations(refresh: () => Promise<void>): ActivityOperations {
  const toast = useToast();

  const retry = useCallback(
    async (entry: ActivityEntry) => {
      try {
        await retryActivity(entry.id);
      } catch (error) {
        toast(t.activity.toast.retryFailed(errorText(error)));
        return;
      }
      await refresh();
    },
    [refresh, toast],
  );

  const undo = useCallback(
    async (entry: ActivityEntry, tagId?: number) => {
      try {
        const written = await undoActivity(entry.id, tagId);
        toast(
          written.outcome === "already_reverted"
            ? t.activity.toast.alreadyReverted
            : t.activity.toast.undone,
        );
      } catch (error) {
        toast(t.activity.toast.undoFailed(errorText(error)));
        return;
      }
      await refresh();
    },
    [refresh, toast],
  );

  return { retry, undo };
}
