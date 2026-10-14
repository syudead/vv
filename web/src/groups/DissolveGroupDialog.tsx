import { CircleAlert } from "lucide-react";
import { useState } from "react";

import { type LibraryGroup, RequestFailed } from "../api/client";
import { dissolveGroup } from "../api/groups";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Spinner } from "../ui/shadcn/spinner";

/**
 * DissolveGroupDialog はグループを解散する確認の窓である（specs/048-groups/ui-design.md
 * 「Dissolving」）。説明は動画とグループがライブラリに残ることを言い、自動グループ（`folder`
 * あり）にはフォルダが自動でまとまらなくなることを 2 文目に足す（要件 13）。
 *
 * 開いている間だけ描く。確定で `DELETE /api/groups/{id}` を送り、送っている間は閉じない。
 * `204` は `onDissolved`、`404`（もう解散されていた）は `onGone` に任せる。ほかの失敗は窓の中の
 * `Alert` に出し、窓を開いたままにする。
 */
export default function DissolveGroupDialog({
  group,
  onClose,
  onDissolved,
  onGone,
}: {
  group: Pick<LibraryGroup, "id" | "name" | "folder">;
  onClose: () => void;
  onDissolved: () => void;
  onGone: () => void;
}) {
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const confirm = () => {
    if (pending) return;
    setPending(true);
    setFailure(null);
    dissolveGroup(group.id).then(
      () => {
        setPending(false);
        onDissolved();
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 404) {
          onGone();
          return;
        }
        setFailure(t.groups.dissolveDialog.failed(errorText(error)));
      },
    );
  };

  const description =
    group.folder === undefined ? (
      t.groups.dissolveDialog.description
    ) : (
      <>
        {t.groups.dissolveDialog.description} {t.groups.dissolveDialog.folder}
      </>
    );

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={t.groups.dissolveDialog.title(group.name)}
      description={description}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.groups.dissolveDialog.submitting : t.groups.dissolveDialog.submit}
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
