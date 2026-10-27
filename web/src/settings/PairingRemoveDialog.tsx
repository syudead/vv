import { useState } from "react";

import { errorText, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";

import { FailureAlert } from "./SaveStatus";

export interface PairingRemoveWords {
  /** The ghost-destructive trigger. */
  trigger: UiText;
  title: UiText;
  description: UiText;
  action: UiText;
  cancel: UiText;
  /** The toast after the request succeeded. */
  done: UiText;
  failed: (reason: string) => UiText;
}

/**
 * The confirmation that removes one direction of a pairing: the link this VVMDM reads, or the access
 * of the VVMDM that reads it. The dialog stays open with the failure until the request succeeds.
 */
export function PairingRemoveDialog({
  words,
  remove,
  onRemoved,
}: {
  words: PairingRemoveWords;
  remove: () => Promise<void>;
  onRemoved: () => void;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const confirm = async () => {
    setPending(true);
    setFailure(null);
    try {
      await remove();
      toast(words.done);
      onRemoved();
    } catch (error) {
      setFailure(words.failed(errorText(error)));
      setPending(false);
    }
  };

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        if (pending) return;
        setOpen(next);
        if (next) setFailure(null);
      }}
      trigger={
        <Button variant="ghost-destructive" size="sm">
          {words.trigger}
        </Button>
      }
      title={words.title}
      description={words.description}
      actionLabel={words.action}
      cancelLabel={words.cancel}
      pending={pending}
      onConfirm={() => void confirm()}
    >
      {failure !== null && <FailureAlert text={failure} />}
    </ConfirmDialog>
  );
}
