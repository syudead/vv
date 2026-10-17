import { CircleAlert, RotateCw } from "lucide-react";
import { useEffect, useState } from "react";

import { deleteSmartFolder, listSmartFolders } from "../api/smartFolders";
import GroupConfirmDialog from "../groups/GroupConfirmDialog";
import { errorText, t, type UiText } from "../i18n";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { descendantIds } from "./smartRule";

/**
 * useDescendantCount は `id` の子孫（どの深さも）の数を `GET /api/smart-folders` の木から数える。
 * 数え終わるまで `count` は undefined、失敗したら `failure` に理由を持つ。`retry` で数え直す。
 */
function useDescendantCount(id: number) {
  const [count, setCount] = useState<number | undefined>();
  const [failure, setFailure] = useState<UiText | null>(null);
  const [round, setRound] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    listSmartFolders(controller.signal).then(
      (folders) => setCount(descendantIds(folders, id).size),
      (error: unknown) => {
        if (!controller.signal.aborted) setFailure(errorText(error));
      },
    );
    return () => controller.abort();
  }, [id, round]);
  const retry = () => {
    setFailure(null);
    setRound((value) => value + 1);
  };
  return { count, failure, retry };
}

/**
 * DeleteSmartFolderDialog はスマートフォルダを消す確認の窓である。説明は動画がライブラリに残ることを言い、
 * 子孫があればその数（どの深さも、`GET /api/smart-folders` の木から数える）を 2 文目に足す。
 * 数え終わるまで「Delete」を押せない — 子孫の警告の無いまま子孫ごと消さないため。数えられ
 * なければ窓の中の `Alert` に理由と「Retry」を出し、数え直せるまで押せないままにする。
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
  const descendants = useDescendantCount(folder.id);
  const count = descendants.count;

  return (
    <GroupConfirmDialog
      request={() => deleteSmartFolder(folder.id)}
      onDone={() => onDeleted(true)}
      onClose={onClose}
      onGone={() => onDeleted(false)}
      title={words.title(folder.name)}
      description={
        count === undefined || count === 0 ? (
          words.description
        ) : (
          <>
            {words.description} {words.descendants(count)}
          </>
        )
      }
      actionDisabled={count === undefined}
      notice={
        descendants.failure !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{t.smartFolders.listFailed(descendants.failure)}</AlertTitle>
            <AlertAction>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={descendants.retry}
              >
                <RotateCw aria-hidden="true" />
                {t.common.retry}
              </Button>
            </AlertAction>
          </Alert>
        )
      }
      submit={words.submit}
      submitting={words.submitting}
      failed={words.failed}
    />
  );
}
