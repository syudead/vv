import { CircleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";

import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Spinner } from "../ui/shadcn/spinner";

/**
 * Mount only while open and block closing during requests; callers own success navigation and
 * notifications. Delegate 404 to onGone and show other failures inline.
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
  /** Disable confirmation until required impact data is available. */
  actionDisabled?: boolean;
  /** Content between the description and submission errors, such as impact-loading failures. */
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
