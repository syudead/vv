import { useId, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  confirmPeerCertificate,
  type PeerLinkState,
  type PeerState,
  unpairPeer,
  updatePeerAddress,
} from "../api/peer";
import { errorText, t, type UiText } from "../i18n";
import { FactList } from "../ui/patterns/fact-list";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";

import { PairingRemoveDialog } from "./PairingRemoveDialog";
import { PeerLinkBadge } from "./PeerLinkBadge";
import { FingerprintAlert } from "./PairingSectionFingerprint";
import { FailureAlert } from "./SaveStatus";
import { TextSettingRow, useTextSettingSave } from "./TextSettingRow";

/** Why a link is not connected and what to do; a connected link needs no line. */
function stateDetail(link: PeerLinkState): UiText | undefined {
  const text = t.settings.pairing.state;
  if (link.state === "connected") return undefined;
  if (link.state === "revoked") return text.revokedDetail;
  // Without a pinned certificate nothing was trusted before, so nothing "changed".
  if (link.reason === "certificate")
    return link.pinned === true ? text.certificateDetail : text.untrustedDetail;
  return text.networkDetail;
}

/** Saved on blur or Enter; the answer updates the link's details, the next peer event the state. */
function AddressRow({
  nodeId,
  address,
  onSaved,
}: {
  nodeId: string;
  address: string;
  onSaved: (state: PeerState) => void;
}) {
  const text = t.settings.pairing.link;
  const id = useId();
  const setting = useTextSettingSave({
    initial: address,
    save: async (next) => {
      const answer = await updatePeerAddress(nodeId, next);
      onSaved(answer);
      return answer.links.find((link) => link.nodeId === nodeId)?.address ?? next;
    },
    isInvalid: (failure) =>
      failure instanceof RequestFailed && failure.reason === "invalid_address",
    failed: text.saveFailed,
  });
  return (
    <TextSettingRow
      id={id}
      label={text.address}
      description={text.addressDescription}
      invalidText={t.settings.pairing.invalidAddress}
      savingText={text.saving}
      savedText={text.saved}
      busyWhileSaving
      setting={setting}
    />
  );
}

/**
 * Sends the fingerprint on screen; fingerprint_changed replaces it with the one now waiting. An
 * unpinned link gets the first-trust wording that pairing uses.
 */
function CertificateRow({
  nodeId,
  address,
  fingerprint,
  pinned,
}: {
  nodeId: string;
  address: string;
  fingerprint: string;
  pinned: boolean;
}) {
  const text = t.settings.pairing;
  const [shown, setShown] = useState(fingerprint);
  const [changed, setChanged] = useState(false);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const trust = async () => {
    setPending(true);
    setFailure(null);
    try {
      await confirmPeerCertificate(nodeId, shown);
    } catch (error) {
      if (
        error instanceof RequestFailed &&
        error.reason === "fingerprint_changed" &&
        error.fingerprint !== undefined
      ) {
        setShown(error.fingerprint);
        setChanged(true);
      } else {
        setFailure(text.link.certificateFailed(errorText(error)));
      }
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <FingerprintAlert
        title={
          changed
            ? text.link.certificateChanged
            : pinned
              ? text.link.certificateTitle
              : text.form.untrustedTitle
        }
        address={address}
        fingerprint={shown}
        action={text.link.trust}
        pending={pending}
        destructive={changed}
        onTrust={() => void trust()}
      />
      {failure !== null && <FailureAlert text={failure} />}
    </div>
  );
}

function UnpairAction({
  nodeId,
  name,
  onUnpaired,
}: {
  nodeId: string;
  name: string;
  onUnpaired: () => void;
}) {
  const text = t.settings.pairing.unpairDialog;
  return (
    <PairingRemoveDialog
      words={{
        trigger: t.settings.pairing.link.unpair,
        title: text.title(name),
        description: text.description,
        action: text.action,
        cancel: t.common.cancel,
        done: text.done(name),
        failed: text.failed,
      }}
      remove={() => unpairPeer(nodeId)}
      onRemoved={onUnpaired}
    />
  );
}

/** Revoked needs no confirmation: the other side already withdrew the access. */
function RemoveAction({ nodeId, onRemoved }: { nodeId: string; onRemoved: () => void }) {
  const text = t.settings.pairing.link;
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const remove = async () => {
    setPending(true);
    setFailure(null);
    try {
      await unpairPeer(nodeId);
      onRemoved();
    } catch (error) {
      setFailure(text.removeFailed(errorText(error)));
      setPending(false);
    }
  };
  return (
    <div className="flex flex-col items-start gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={pending}
        onClick={() => void remove()}
      >
        {text.remove}
      </Button>
      {failure !== null && <FailureAlert text={failure} />}
    </div>
  );
}

/**
 * The section of one VVMDM this one reads, titled with its name: the state's detail while it is not
 * connected, its version, address, certificate and removal. The description says what a connected
 * link shows; the detail takes its place otherwise, so the header never contradicts the state.
 */
export function LinkSection({
  link,
  onLinkAnswer,
  onUnpaired,
}: {
  link: PeerLinkState;
  onLinkAnswer: (state: PeerState) => void;
  onUnpaired: () => void;
}) {
  const text = t.settings.pairing.link;
  const name = link.name ?? "";
  const nodeId = link.nodeId ?? "";
  const revoked = link.state === "revoked";
  const detail = stateDetail(link);
  const certificate =
    link.state === "unreachable" && link.reason === "certificate"
      ? link.fingerprint
      : undefined;
  return (
    <PageSection
      title={name}
      description={detail === undefined ? text.description : undefined}
      actions={<PeerLinkBadge state={link.state} />}
    >
      {detail !== undefined && <p className="text-muted-foreground">{detail}</p>}
      <FactList
        facts={[{ id: "version", term: text.version, value: link.version ?? "" }]}
      />
      {!revoked && link.address !== undefined && (
        <AddressRow nodeId={nodeId} address={link.address} onSaved={onLinkAnswer} />
      )}
      {certificate !== undefined && (
        <CertificateRow
          key={certificate}
          nodeId={nodeId}
          address={link.address ?? ""}
          fingerprint={certificate}
          pinned={link.pinned === true}
        />
      )}
      <div>
        {revoked ? (
          <RemoveAction nodeId={nodeId} onRemoved={onUnpaired} />
        ) : (
          <UnpairAction nodeId={nodeId} name={name} onUnpaired={onUnpaired} />
        )}
      </div>
    </PageSection>
  );
}
