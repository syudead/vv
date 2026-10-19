import type { Credit, CreditKind } from "../api/credits";
import { t, type UiText } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { creditDisplayName } from "./displayName";

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
  const name = creditDisplayName(credit);
  const text = t.credits.deleteDialog.text(name, credit.videoCount);
  const note =
    kind === "organization"
      ? t.credits.deleteDialog.organizationNote
      : t.credits.deleteDialog.personNote;
  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t.credits.deleteDialog.title(name)}
      description={
        <>
          {text} {note}
        </>
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
