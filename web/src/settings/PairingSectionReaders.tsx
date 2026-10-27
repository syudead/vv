import { BookOpen } from "lucide-react";

import { type PeerGrant, revokePeerGrant } from "../api/peer";
import { formatDateTime, t } from "../i18n";
import { PageSection } from "../ui/patterns/page-section";

import { PairingRemoveDialog } from "./PairingRemoveDialog";
import { PairingCodeRow } from "./PairingSectionCode";

function RemoveAccessAction({
  name,
  onRevoked,
}: {
  name: string;
  onRevoked: () => void;
}) {
  const text = t.settings.pairing.readers.removeDialog;
  return (
    <PairingRemoveDialog
      words={{
        trigger: t.settings.pairing.readers.remove,
        title: text.title(name),
        description: text.description,
        action: text.action,
        cancel: t.common.cancel,
        done: text.done(name),
        failed: text.failed,
      }}
      remove={() => revokePeerGrant()}
      onRemoved={onRevoked}
    />
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
