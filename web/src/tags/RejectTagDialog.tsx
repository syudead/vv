import type { Tag } from "../api/tags";
import { t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { DialogError } from "./DialogError";

/**
 * RejectTagDialog は仮のタグの却下の確認の窓である
 * （specs/031-tentative-tags/ui-design.md「Reject」）。骨格は削除の窓
 * （DeleteTagDialog）と同じ `ConfirmDialog` で、本文だけが「却下した名前から戻せる」
 * ことを知らせる。本数は `GET /api/tags` の `videoCount`。
 */
export default function RejectTagDialog({
  tag,
  pending,
  error,
  onClose,
  onReject,
}: {
  tag: Tag;
  pending: boolean;
  error: UiText | null;
  onClose: () => void;
  onReject: () => void;
}) {
  const message =
    tag.videoCount === 0
      ? t.tags.rejectDialog.unused(tag.name)
      : t.tags.rejectDialog.used(tag.name, tag.videoCount);

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t.tags.rejectDialog.title(tag.name)}
      description={message}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.tags.rejectDialog.submitting : t.tags.rejectDialog.submit}
        </>
      }
      onConfirm={onReject}
      pending={pending}
    >
      {error !== null && <DialogError message={error} />}
    </ConfirmDialog>
  );
}
