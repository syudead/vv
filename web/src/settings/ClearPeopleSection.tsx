import { LoaderCircle } from "lucide-react";
import { useState } from "react";

import { clearPeople } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";

/** Deletes every person and all face data after a confirmation, from the Danger zone page. */
export default function ClearPeopleSection() {
  const text = t.settings.dangerZone.clearPeople;
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  const confirm = () => {
    setPending(true);
    setError(null);
    clearPeople().then(
      (cleared) => {
        setPending(false);
        setOpen(false);
        toast(text.done(cleared.deleted, cleared.queued));
      },
      (failure: unknown) => {
        setPending(false);
        setError(text.failed(errorText(failure)));
      },
    );
  };

  return (
    <PageSection title={text.heading} description={text.description}>
      <div>
        <Button
          size="sm"
          variant="ghost-destructive"
          onClick={() => {
            setError(null);
            setOpen(true);
          }}
        >
          {text.open}
        </Button>
      </div>
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          if (!pending) setOpen(next);
        }}
        title={text.title}
        description={text.body}
        cancelLabel={t.common.cancel}
        actionLabel={
          <>
            {pending && (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            )}
            {pending ? text.submitting : text.submit}
          </>
        }
        pending={pending}
        onConfirm={confirm}
      >
        {error !== null && <DialogError message={error} />}
      </ConfirmDialog>
    </PageSection>
  );
}
