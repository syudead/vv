import { useCallback, useEffect, useRef, useState } from "react";

import { renameGroup } from "../api/groups";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";

/** GroupNaming は名前の欄を開いている新しいグループである。 */
export interface GroupNaming {
  groupId: number;
  /** サーバーが付けた仮の名前。 */
  provisional: string;
  /** そのグループを作った「Make a group」の取り消しのトークン。 */
  undoToken: string;
}

/**
 * useGroupNaming は「Make a group」の後の名前の欄である（specs/050-drag-organize/ui-design.md
 * 「The new group's name」）。変えた名前は `onShowName` ですぐ見せてから、作ったときのトークン付きの
 * PATCH /api/groups/{id} で送る。失敗したら「Couldn't rename the group: {reason}」を出し、仮の名前を
 * 見せ直す。欄が閉じたら、送っている名前が答えてから `onClosed` を呼ぶ（画面はそこで読み直す）。
 */
export function useGroupNaming({
  onShowName,
  onClosed,
}: {
  onShowName: (groupId: number, name: string) => void;
  onClosed: () => void;
}) {
  const toast = useToast();
  const [naming, setNaming] = useState<GroupNaming | null>(null);
  const sending = useRef<Promise<void>>(Promise.resolve());
  const open = useRef(false);
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
    open.current = true;
    setNaming(next);
  }, []);

  const close = useCallback(() => {
    if (!open.current) return;
    open.current = false;
    setNaming(null);
    void sending.current.then(() => latest.current.onClosed());
  }, []);

  return { naming, start, rename, close };
}
