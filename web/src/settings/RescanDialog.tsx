import { CircleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";

import type { StartRescanRequest } from "../api/client";
import { t, type UiText } from "../i18n";
import { useScanControls } from "../shell/ScanProvider";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";

/** Mount only while open; it closes on a started rescan and keeps a failure inside it. */
export default function RescanDialog({
  request,
  title,
  description,
  children,
  onClose,
}: {
  request: StartRescanRequest;
  title: UiText;
  description: UiText;
  children?: ReactNode;
  onClose: () => void;
}) {
  const { rescan } = useScanControls();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const text = t.settings.scanStatus.rescanDialog;

  const confirm = () => {
    setPending(true);
    setError(null);
    void rescan(request).then((failure) => {
      setPending(false);
      if (failure === null) onClose();
      else setError(t.shell.scan.rescanFailed(failure));
    });
  };

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={title}
      description={description}
      actionLabel={pending ? text.starting : text.submit}
      cancelLabel={t.common.cancel}
      onConfirm={confirm}
      pending={pending}
    >
      {children}
      {error !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle className="break-words">{error}</AlertTitle>
        </Alert>
      )}
    </ConfirmDialog>
  );
}
