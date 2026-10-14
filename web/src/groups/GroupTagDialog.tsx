import { CircleAlert } from "lucide-react";
import { useState } from "react";

import { RequestFailed } from "../api/client";
import { type GroupTagResult, tagGroup } from "../api/groups";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Spinner } from "../ui/shadcn/spinner";

/**
 * GroupTagDialog はグループをタグに変える確認の窓である（specs/048-groups/ui-design.md
 * 「Turning into a tag」、research.md R-11）。グループページの「More」、動画ページのグループ操作、
 * フォルダのまとめ方のメニューの 3 つの入り口が、同じ文のこの窓を開く。
 *
 * 開いている間だけ描く。確定で `POST /api/groups/{id}/tag` を送り、送っている間は閉じない。
 * `200` は `onTagged` に結果を渡し（閉じるのと、トーストと、画面の動きは呼び出し側が持つ）、
 * `404`（途中で解散された）は `onGone` に任せる。ほかの失敗（`400 invalid_request` はタグ名の規則が
 * 理由）は窓の中の `Alert` に出し、窓を開いたままにする。
 */
export default function GroupTagDialog({
  group,
  onClose,
  onTagged,
  onGone,
}: {
  group: { id: number; name: string };
  onClose: () => void;
  onTagged: (result: GroupTagResult) => void;
  onGone: () => void;
}) {
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const confirm = () => {
    if (pending) return;
    setPending(true);
    setFailure(null);
    tagGroup(group.id).then(
      (result) => {
        setPending(false);
        onTagged(result);
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 404) {
          onGone();
          return;
        }
        setFailure(t.groups.tagDialog.failed(errorText(error)));
      },
    );
  };

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={t.groups.tagDialog.title(group.name)}
      description={t.groups.tagDialog.description(group.name)}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.groups.tagDialog.submitting : t.groups.tagDialog.submit}
        </>
      }
      onConfirm={confirm}
      pending={pending}
    >
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </ConfirmDialog>
  );
}
