import { useState } from "react";

import { isCreditGone } from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { formatBytes } from "../lib/format";
import { UPLOAD_MAX_BYTES, uploadRefusal } from "../player/thumbnailUpload/uploadRules";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Spinner } from "../ui/shadcn/spinner";

// The parts a person's face and an organization's logo share on their pages.

const face = t.credits.personPage.face;

/** Why an upload dialog refuses this file, or null when it can be saved. */
export function imageRefusal(file: File): UiText | null {
  const refusal = uploadRefusal(file);
  if (refusal === "type") return face.refusedType;
  if (refusal === "size") return face.tooLarge(formatBytes(UPLOAD_MAX_BYTES));
  return null;
}

/** Take only the image from an image response, so a heart pressed meanwhile keeps its state. */
export function withImageOf<T extends { imageRevision?: number }>(saved: T) {
  return (current: T): T => ({ ...current, imageRevision: saved.imageRevision });
}

/**
 * Asks before removing the image and stays open while it is removed. A failure stays in the
 * dialog unless onFailed takes its reason, and a missing record calls onGone.
 */
export function RemoveImageDialog({
  title,
  onRemove,
  onClose,
  onGone,
  onFailed,
}: {
  title: UiText;
  onRemove: () => Promise<void>;
  onClose: () => void;
  onGone: () => void;
  /** Called with the reason in place of showing it, after the dialog closes. */
  onFailed?: (reason: UiText) => void;
}) {
  const [removing, setRemoving] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  async function remove() {
    setError(null);
    setRemoving(true);
    try {
      await onRemove();
      onClose();
    } catch (failure) {
      if (isCreditGone(failure)) {
        onGone();
        return;
      }
      if (onFailed !== undefined) {
        onClose();
        onFailed(errorText(failure));
        return;
      }
      setError(errorText(failure));
    } finally {
      setRemoving(false);
    }
  }

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !removing) onClose();
      }}
      title={title}
      description={face.removeText}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {removing && <Spinner aria-hidden="true" />}
          {removing ? face.removing : face.removeSubmit}
        </>
      }
      onConfirm={() => void remove()}
      pending={removing}
    >
      {error !== null && <DialogError message={error} />}
    </ConfirmDialog>
  );
}
