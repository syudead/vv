import { useEffect, useState } from "react";

import { deleteSmartFolder, listSmartFolders } from "../api/smartFolders";
import GroupConfirmDialog from "../groups/GroupConfirmDialog";
import { t } from "../i18n";
import { descendantIds } from "./smartRule";

/**
 * DeleteSmartFolderDialog はスマートフォルダを消す確認の窓である
 * （specs/052-smart-folders/ui-design.md「Deleting」）。説明は動画がライブラリに残ることを言い、
 * 子孫があればその数（どの深さも、`GET /api/smart-folders` の木から数える）を 2 文目に足す。
 *
 * 開いている間だけ描く。確定で `DELETE /api/smart-folders/{id}` を送り、送っている間は閉じない。
 * `204` は `onDeleted(true)`、`404`（もう無かった）は `onDeleted(false)`（トーストを出さない）。
 * ほかの失敗は窓の中の `Alert` に出し、窓を開いたままにする（GroupConfirmDialog）。
 */
export default function DeleteSmartFolderDialog({
  folder,
  onClose,
  onDeleted,
}: {
  folder: { id: number; name: string };
  onClose: () => void;
  /** 消えた。`announce` が偽なら、もう無かった。 */
  onDeleted: (announce: boolean) => void;
}) {
  const words = t.smartFolders.deleteDialog;
  const [descendants, setDescendants] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    listSmartFolders(controller.signal).then(
      (folders) => setDescendants(descendantIds(folders, folder.id).size),
      () => undefined,
    );
    return () => controller.abort();
  }, [folder.id]);

  return (
    <GroupConfirmDialog
      request={() => deleteSmartFolder(folder.id)}
      onDone={() => onDeleted(true)}
      onClose={onClose}
      onGone={() => onDeleted(false)}
      title={words.title(folder.name)}
      description={
        descendants === 0 ? (
          words.description
        ) : (
          <>
            {words.description} {words.descendants(descendants)}
          </>
        )
      }
      submit={words.submit}
      submitting={words.submitting}
      failed={words.failed}
    />
  );
}
