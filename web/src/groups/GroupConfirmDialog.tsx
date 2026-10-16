import { CircleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";

import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Spinner } from "../ui/shadcn/spinner";

/**
 * GroupConfirmDialog はグループへの書き込みを 1 回送る確認の窓である。解散（DissolveGroupDialog）と
 * タグ化（GroupTagDialog）が、要求と文言だけを渡して使う。
 *
 * 開いている間だけ描く。確定で `request` を送り、送っている間は閉じない。成功は `onDone` に
 * 結果を渡し（閉じるのと、トーストと、画面の動きは呼び出し側が持つ）、`404`（グループがもう無い）は
 * `onGone` に任せる。ほかの失敗は `failed` の文で窓の中の `Alert` に出し、窓を開いたままにする。
 * `actionDisabled` と `notice` で、確定の前に要るもの（影響の数など）が揃うまで確定を止められる。
 */
export default function GroupConfirmDialog<T>({
  request,
  onDone,
  onClose,
  onGone,
  title,
  description,
  submit,
  submitting,
  failed,
  actionDisabled,
  notice,
}: {
  request: () => Promise<T>;
  onDone: (result: T) => void;
  onClose: () => void;
  onGone: () => void;
  title: ReactNode;
  description: ReactNode;
  submit: ReactNode;
  submitting: ReactNode;
  failed: (reason: UiText) => UiText;
  /** 確定をまだ押せない（影響を数えている間や、数えられなかったとき）。 */
  actionDisabled?: boolean;
  /** 説明の下、送った結果の失敗の上に置くもの（数えられなかったときの Alert など）。 */
  notice?: ReactNode;
}) {
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const confirm = () => {
    if (pending) return;
    setPending(true);
    setFailure(null);
    request().then(
      (result) => {
        setPending(false);
        onDone(result);
      },
      (error: unknown) => {
        setPending(false);
        if (error instanceof RequestFailed && error.status === 404) {
          onGone();
          return;
        }
        setFailure(failed(errorText(error)));
      },
    );
  };

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={title}
      description={description}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? submitting : submit}
        </>
      }
      onConfirm={confirm}
      pending={pending}
      actionDisabled={actionDisabled}
    >
      {notice}
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </ConfirmDialog>
  );
}
