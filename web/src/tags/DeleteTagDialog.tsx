import type { Tag } from "../api/tags";
import { t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { DialogError } from "./DialogError";

/**
 * DeleteTagDialog はタグ削除の確認の窓である（ui-design.md「Merge and
 * delete」、受け入れ条件 11）。本数は `GET /api/tags` の `videoCount`。
 * デザインシステムの `ConfirmDialog`（`AlertDialog`）に載せ、送っている間は閉じない。
 */
export default function DeleteTagDialog({
  tag,
  pending,
  error,
  onClose,
  onDelete,
}: {
  tag: Tag;
  pending: boolean;
  error: UiText | null;
  onClose: () => void;
  onDelete: () => void;
}) {
  const message =
    tag.videoCount === 0
      ? t.tags.deleteDialog.unused
      : t.tags.deleteDialog.used(tag.videoCount);

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t.tags.deleteDialog.title(tag.name)}
      description={message}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.tags.deleteDialog.submitting : t.tags.deleteDialog.submit}
        </>
      }
      onConfirm={onDelete}
      pending={pending}
    >
      {error !== null && <DialogError message={error} />}
    </ConfirmDialog>
  );
}
