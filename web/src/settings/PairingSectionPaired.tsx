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
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { FactList } from "../ui/patterns/fact-list";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";

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
  if (state.reason === "certificate")
    return {
      icon: ShieldAlert,
      word: text.certificate,
      detail: text.certificateDetail,
      tone: "warning",
    };
  if (state.reason === "version")
    return {
      icon: Unplug,
      word: text.version,
      detail: text.versionDetail(state.peer?.version ?? "", state.node?.version ?? ""),
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

/** Saved on blur or Enter; the answer updates the peer's details, the next peer event the state. */
function AddressRow({
  address,
  onSaved,
}: {
  address: string;
  onSaved: (state: PeerState) => void;
}) {
  const text = t.settings.pairing.paired;
  const id = useId();
  const setting = useTextSettingSave({
    initial: address,
    save: async (next) => {
      const answer = await updatePeerAddress(next);
      onSaved(answer);
      return answer.peer?.address ?? next;
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

/** Sends the fingerprint on screen; fingerprint_changed replaces it with the one now waiting. */
function CertificateRow({
  address,
  fingerprint,
}: {
  address: string;
  fingerprint: string;
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
        setFailure(text.paired.certificateFailed(errorText(error)));
      }
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <FingerprintAlert
        title={changed ? text.paired.certificateChanged : text.paired.certificateTitle}
        address={address}
        fingerprint={shown}
        action={text.paired.trust}
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
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  const unpair = async () => {
    setPending(true);
    setFailure(null);
    try {
      await unpairPeer();
      toast(text.done(name));
      onUnpaired();
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
          {t.settings.pairing.paired.unpair}
        </Button>
      }
      title={text.title(name)}
      description={text.description}
      actionLabel={text.action}
      cancelLabel={t.common.cancel}
      pending={pending}
      onConfirm={() => void unpair()}
    >
      {failure !== null && <FailureAlert text={failure} />}
    </ConfirmDialog>
  );
}

/** Revoked needs no confirmation: the other side already ended the pairing. */
function RemoveAction({ onRemoved }: { onRemoved: () => void }) {
  const text = t.settings.pairing.paired;
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

export function PairedSection({
  state,
  onPeerAnswer,
  onUnpaired,
}: {
  state: PeerState;
  onPeerAnswer: (state: PeerState) => void;
  onUnpaired: () => void;
}) {
  const text = t.settings.pairing.paired;
  const peer = state.peer;
  const name = peer?.name ?? "";
  const revoked = state.state === "revoked";
  const certificate =
    state.state === "unreachable" && state.reason === "certificate"
      ? state.fingerprint
      : undefined;
  return (
    <PageSection title={text.heading} description={text.description}>
      <StateRow state={state} />
      <FactList
        facts={[
          { id: "name", term: text.name, value: name },
          {
            id: "version",
            term: text.version,
            value: (
              <span className={cn(state.reason === "version" && "text-warning")}>
                {peer?.version ?? ""}
              </span>
            ),
          },
        ]}
      />
      {!revoked && peer !== undefined && (
        <AddressRow address={peer.address} onSaved={onPeerAnswer} />
      )}
      {certificate !== undefined && (
        <CertificateRow
          key={certificate}
          address={peer?.address ?? ""}
          fingerprint={certificate}
        />
      )}
      <div>
        {revoked ? (
          <RemoveAction onRemoved={onUnpaired} />
        ) : (
          <UnpairAction name={name} onUnpaired={onUnpaired} />
        )}
      </div>
    </PageSection>
  );
}
