import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";

/** Keep open while submitting and show failures within the dialog. */
export default function WorkInfoPluginDialog({
  title,
  description,
  submitLabel,
  pending,
  submitDisabled,
  failure,
  onClose,
  onSubmit,
  children,
}: {
  title: UiText;
  description: UiText;
  submitLabel: UiText;
  pending: boolean;
  submitDisabled: boolean;
  failure: UiText | null;
  onClose: () => void;
  onSubmit: () => void;
  children: ReactNode;
}) {
  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={title}
      description={description}
      submitLabel={submitLabel}
      cancelLabel={t.common.cancel}
      pending={pending}
      submitDisabled={submitDisabled}
      onSubmit={(event) => {
        event.preventDefault();
        if (!pending && !submitDisabled) onSubmit();
      }}
    >
      {children}
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}
