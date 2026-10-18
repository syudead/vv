import { useCallback } from "react";

import { type ActivityEntry, retryActivity, undoActivity } from "../api/activity";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";

export interface ActivityOperations {
  retry: (entry: ActivityEntry) => Promise<void>;
  undo: (entry: ActivityEntry, tagId?: number) => Promise<void>;
}

/**
 * Resolve successful operations only after refreshing the list, preventing duplicate actions on
 * stale cards. Report failures without rejecting the promise.
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
