import { useCallback, useEffect, useRef, useState } from "react";

import { renameGroup } from "../api/groups";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";

export interface GroupNaming {
  groupId: number;
  provisional: string;
  undoToken: string;
}

/**
 * Rename optimistically with the creation undo token and restore the provisional name on failure.
 * Wait for pending rename before onClosed, passing that closed field even if another opened;
 * discard skips onClosed.
 */
export function useGroupNaming({
  onShowName,
  onClosed,
}: {
  onShowName: (groupId: number, name: string) => void;
  onClosed: (closed: GroupNaming) => void;
}) {
  const toast = useToast();
  const [naming, setNaming] = useState<GroupNaming | null>(null);
  const sending = useRef<Promise<void>>(Promise.resolve());
  const open = useRef<GroupNaming | null>(null);
  const latest = useRef({ onShowName, onClosed });
  useEffect(() => {
    latest.current = { onShowName, onClosed };
  });

  const rename = useCallback(
    (name: string) => {
      if (naming === null) return;
      const { groupId, provisional, undoToken } = naming;
      latest.current.onShowName(groupId, name);
      sending.current = renameGroup(groupId, name, { undoToken }).then(
        () => undefined,
        (error: unknown) => {
          toast(t.groups.renameDialog.failed(errorText(error)));
          latest.current.onShowName(groupId, provisional);
        },
      );
    },
    [naming, toast],
  );

  const start = useCallback((next: GroupNaming) => {
    open.current = next;
    setNaming(next);
  }, []);

  const close = useCallback(() => {
    const closed = open.current;
    if (closed === null) return;
    open.current = null;
    setNaming(null);
    void sending.current.then(() => latest.current.onClosed(closed));
  }, []);

  const discard = useCallback(() => {
    open.current = null;
    setNaming(null);
  }, []);

  return { naming, start, rename, close, discard };
}
