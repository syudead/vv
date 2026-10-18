import type { Credit, CreditKind } from "../api/credits";
import { t, type UiText } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Spinner } from "../ui/shadcn/spinner";

/** Keep the confirmation dialog open while submitting. */
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
