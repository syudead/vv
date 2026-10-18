import { useCallback, useEffect, useRef } from "react";

import { RequestFailed } from "../api/client";
import { undoOrganize } from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";

// Keep one eight-second Undo notification; each accepted operation replaces the previous one.

export function undoFailedText(error: unknown): UiText {
  if (error instanceof RequestFailed) {
    if (error.status === 409) return t.organize.undoFailed.conflict;
    if (error.status === 410) return t.organize.undoFailed.expired;
  }
  return t.organize.undoFailed.other(errorText(error));
}

export interface UndoToast {
  /**
   * Call onSettled before reloading so callers can remove optimistic overlays according to undo
   * success.
   */
  offer: (message: UiText, token: string, onSettled?: (undone: boolean) => void) => void;
  /** Call before operation-driven navigation to preserve the current notification and token. */
  carry: () => void;
}

/**
 * Reload after both successful and failed undo; unmount dismisses and forgets the token unless
 * carry was called.
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
    (message: UiText, token: string, onSettled?: (undone: boolean) => void) => {
      carried.current = false;
      dismiss.current = toast(message, {
        action: {
          label: t.organize.undo,
          run: async () => {
            let undone = false;
            try {
              await undoOrganize(token);
              undone = true;
              toast(t.organize.undone);
            } catch (error) {
              toast(undoFailedText(error));
            }
            dismiss.current = null;
            onSettled?.(undone);
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
