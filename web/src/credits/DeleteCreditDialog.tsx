import type { Credit, CreditKind } from "../api/credits";
import { t, type UiText } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Spinner } from "../ui/shadcn/spinner";

/**
 * DeleteCreditDialog は人物・団体の削除の確認の窓である。本文に一覧の `videoCount` を出し（Issue の Edge Case
 * 「削除で件数を示す」）、団体では中の団体が親を失うことも書く。`ConfirmDialog`
 * （`AlertDialog`）に載せ、送っている間は閉じない。
 */
export default function DeleteCreditDialog({
  kind,
  credit,
  pending,
  error,
  onClose,
  onDelete,
}: {
  kind: CreditKind;
  credit: Credit;
  pending: boolean;
  error: UiText | null;
  onClose: () => void;
  onDelete: () => void;
}) {
  const text = t.credits.deleteDialog.text(credit.name, credit.videoCount);
  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t.credits.deleteDialog.title(credit.name)}
      description={
        kind === "organization" ? (
          <>
            {text} {t.credits.deleteDialog.organizationNote}
          </>
        ) : (
          text
        )
      }
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.credits.deleteDialog.submitting : t.credits.deleteDialog.submit}
        </>
      }
      onConfirm={onDelete}
      pending={pending}
    >
      {error !== null && <DialogError message={error} />}
    </ConfirmDialog>
  );
}
