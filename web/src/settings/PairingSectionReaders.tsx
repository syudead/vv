import { BookOpen } from "lucide-react";
import { useState } from "react";

import { type PeerGrant, revokePeerGrant } from "../api/peer";
import { errorText, formatDateTime, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";

import { PairingCodeRow } from "./PairingSectionCode";
import { FailureAlert } from "./SaveStatus";

function RemoveAccessAction({
  name,
  onRevoked,
}: {
  name: string;
  onRevoked: () => void;
}) {
  const text = t.settings.pairing.readers.removeDialog;
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const revoke = async () => {
    setPending(true);
    setFailure(null);
    try {
      await revokePeerGrant();
      toast(text.done(name));
      onRevoked();
    } catch (error) {
      setFailure(text.failed(errorText(error)));
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
          {t.settings.pairing.readers.remove}
        </Button>
      }
      title={text.title(name)}
      description={text.description}
      actionLabel={text.action}
      cancelLabel={t.common.cancel}
      pending={pending}
      onConfirm={() => void revoke()}
    >
      {failure !== null && <FailureAlert text={failure} />}
    </ConfirmDialog>
  );
}

/** The one VVMDM allowed to read this one, with when it paired and the way to withdraw its access. */
function GrantRow({ grant, onRevoked }: { grant: PeerGrant; onRevoked: () => void }) {
  const text = t.settings.pairing.readers;
  return (
    <div className="flex min-w-0 items-start gap-3">
      <BookOpen aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-success" />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-sm font-medium break-words">{text.reads(grant.name)}</p>
        <p className="text-xs text-muted-foreground tabular-nums">
          {text.pairedAt(formatDateTime(grant.pairedAt))}
        </p>
      </div>
      <RemoveAccessAction name={grant.name} onRevoked={onRevoked} />
    </div>
  );
}

/**
 * The VVMDM that reads this one. Without a grant the section offers the pairing code; with one, the
 * code would be refused, so the grant takes its place.
 */
export function ReadersSection({
  grant,
  onRevoked,
}: {
  grant: PeerGrant | undefined;
  onRevoked: () => void;
}) {
  const text = t.settings.pairing.readers;
  return (
    <PageSection title={text.heading} description={text.description}>
      {grant === undefined ? (
        <PairingCodeRow />
      ) : (
        <GrantRow grant={grant} onRevoked={onRevoked} />
      )}
    </PageSection>
  );
}
