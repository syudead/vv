import { CircleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";

import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";

/** reload marks a content_changed failure the caller can answer by re-reading. */
interface Failure {
  text: UiText;
  reload: boolean;
}

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
  changed,
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
  /** Text and Reload for 409 content_changed, when the request carries the version the screen read. */
  changed?: { text: UiText; onReload: () => void };
}) {
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<Failure | null>(null);

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
        if (
          changed !== undefined &&
          error instanceof RequestFailed &&
          error.reason === "content_changed"
        ) {
          setFailure({ text: changed.text, reload: true });
          return;
        }
        setFailure({ text: failed(errorText(error)), reload: false });
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
          <AlertTitle>{failure.text}</AlertTitle>
          {failure.reload && changed !== undefined && (
            <AlertAction>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={changed.onReload}
              >
                {t.common.reload}
              </Button>
            </AlertAction>
          )}
        </Alert>
      )}
    </ConfirmDialog>
  );
}
