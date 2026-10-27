import { Link, type LucideIcon, ShieldAlert, Unlink, Unplug } from "lucide-react";
import { useId, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  confirmPeerCertificate,
  type PeerState,
  unpairPeer,
  updatePeerAddress,
} from "../api/peer";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { FactList } from "../ui/patterns/fact-list";
import { Button } from "../ui/shadcn/button";

import { PairingRemoveDialog } from "./PairingRemoveDialog";
import { FingerprintAlert } from "./PairingSectionFingerprint";
import { FailureAlert } from "./SaveStatus";
import { TextSettingRow, useTextSettingSave } from "./TextSettingRow";

interface StateLine {
  icon: LucideIcon;
  word: UiText;
  detail?: UiText;
  tone: "success" | "warning";
}

function stateLine(state: PeerState): StateLine {
  const text = t.settings.pairing.state;
  if (state.state === "connected")
    return { icon: Link, word: text.connected, tone: "success" };
  if (state.state === "revoked")
    return {
      icon: Unlink,
      word: text.revoked,
      detail: text.revokedDetail,
      tone: "warning",
    };
  // Without a pinned certificate nothing was trusted before, so nothing "changed".
  if (state.reason === "certificate")
    return {
      icon: ShieldAlert,
      word: state.link?.pinned === true ? text.certificate : text.untrusted,
      detail: state.link?.pinned === true ? text.certificateDetail : text.untrustedDetail,
      tone: "warning",
    };
  return {
    icon: Unplug,
    word: text.network,
    detail: text.networkDetail,
    tone: "warning",
  };
}

function StateRow({ state }: { state: PeerState }) {
  const line = stateLine(state);
  const Icon = line.icon;
  return (
    <p className="flex flex-col gap-1">
      <span
        className={cn(
          "flex items-center gap-2 text-sm font-medium",
          line.tone === "success" ? "text-success" : "text-warning",
        )}
      >
        <Icon aria-hidden="true" className="size-4" />
        {line.word}
      </span>
      {line.detail !== undefined && (
        <span className="text-sm text-muted-foreground">{line.detail}</span>
      )}
    </p>
  );
}

/** Saved on blur or Enter; the answer updates the link's details, the next peer event the state. */
function AddressRow({
  address,
  onSaved,
}: {
  address: string;
  onSaved: (state: PeerState) => void;
}) {
  const text = t.settings.pairing.link;
  const id = useId();
  const setting = useTextSettingSave({
    initial: address,
    save: async (next) => {
      const answer = await updatePeerAddress(next);
      onSaved(answer);
      return answer.link?.address ?? next;
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
  address,
  fingerprint,
  pinned,
}: {
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
      await confirmPeerCertificate(shown);
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

function UnpairAction({ name, onUnpaired }: { name: string; onUnpaired: () => void }) {
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
      remove={() => unpairPeer()}
      onRemoved={onUnpaired}
    />
  );
}

/** Revoked needs no confirmation: the other side already withdrew the access. */
function RemoveAction({ onRemoved }: { onRemoved: () => void }) {
  const text = t.settings.pairing.link;
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const remove = async () => {
    setPending(true);
    setFailure(null);
    try {
      await unpairPeer();
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

/** The face of the VVMDM this one reads: its state, details, address, certificate and removal. */
export function LinkDetails({
  state,
  onLinkAnswer,
  onUnpaired,
}: {
  state: PeerState;
  onLinkAnswer: (state: PeerState) => void;
  onUnpaired: () => void;
}) {
  const text = t.settings.pairing.link;
  const link = state.link;
  const name = link?.name ?? "";
  const revoked = state.state === "revoked";
  const certificate =
    state.state === "unreachable" && state.reason === "certificate"
      ? state.fingerprint
      : undefined;
  return (
    <>
      <div className="flex flex-col gap-3">
        <StateRow state={state} />
        <p className="text-sm text-muted-foreground">{text.description}</p>
      </div>
      <FactList
        facts={[
          { id: "name", term: text.name, value: name },
          { id: "version", term: text.version, value: link?.version ?? "" },
        ]}
      />
      {!revoked && link !== undefined && (
        <AddressRow address={link.address} onSaved={onLinkAnswer} />
      )}
      {certificate !== undefined && (
        <CertificateRow
          key={certificate}
          address={link?.address ?? ""}
          fingerprint={certificate}
          pinned={link?.pinned === true}
        />
      )}
      <div>
        {revoked ? (
          <RemoveAction onRemoved={onUnpaired} />
        ) : (
          <UnpairAction name={name} onUnpaired={onUnpaired} />
        )}
      </div>
    </>
  );
}
