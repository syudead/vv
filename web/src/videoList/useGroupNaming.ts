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
 * useGroupNaming は「Make a group」の後の名前の欄である。変えた名前は `onShowName` ですぐ見せてから、作ったときのトークン付きの
 * PATCH /api/groups/{id} で送る。失敗したら「Couldn't rename the group: {reason}」を出し、仮の名前を
 * 見せ直す。欄が閉じたら、送っている名前が答えてから閉じた欄を渡して `onClosed` を呼ぶ（画面は
 * そこで読み直す）。答えを待つ間に次の欄が開いていても、渡すのは閉じた欄である。
 * `discard` は欄を `onClosed` を呼ばずに閉じる（取り消しでグループが消えた）。
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
  // 開いている欄（閉じたら null）。
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
