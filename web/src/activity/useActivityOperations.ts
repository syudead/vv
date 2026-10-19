import { useCallback } from "react";

import { type ActivityEntry, retryActivity, undoActivity } from "../api/activity";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";

/** A list entry or a pipeline member's summary; an operation needs only its id. */
export type OperationTarget = Pick<ActivityEntry, "id">;

export interface ActivityOperations {
  retry: (entry: OperationTarget) => Promise<void>;
  undo: (entry: OperationTarget, tagId?: number) => Promise<void>;
}

/**
 * Resolve successful operations only after refreshing the list, preventing duplicate actions on
 * stale cards. Report failures without rejecting the promise.
 */
export function useActivityOperations(refresh: () => Promise<void>): ActivityOperations {
  const toast = useToast();

  const retry = useCallback(
    async (entry: OperationTarget) => {
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
    async (entry: OperationTarget, tagId?: number) => {
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
