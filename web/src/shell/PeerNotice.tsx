import { type LucideIcon, ShieldAlert, Unlink, Unplug } from "lucide-react";
import { Link, useLocation } from "react-router";

import type { PeerState } from "../api/peer";
import { useAudience } from "../auth/audience";
import { t, type UiText } from "../i18n";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { usePeerName, usePeerState } from "./PeerProvider";

const pairingPath = "/settings/pairing";

interface NoticeText {
  icon: LucideIcon;
  title: UiText;
  body: UiText;
}

function noticeText(state: PeerState, name: string, owner: boolean): NoticeText {
  const words = t.shell.peer;
  if (!owner) return { icon: Unplug, title: words.guestTitle, body: words.guestBody };
  if (state.state === "revoked")
    return { icon: Unlink, title: words.revokedTitle(name), body: words.revokedBody };
  if (state.reason === "certificate")
    return {
      icon: ShieldAlert,
      title: words.certificateTitle(name),
      body: words.certificateBody,
    };
  return { icon: Unplug, title: words.networkTitle(name), body: words.networkBody };
}

/** Whether the shell draws the notice: the peer is paired but its items are hidden. */
export function usePeerNoticeShown(): boolean {
  const state = usePeerState();
  const { pathname } = useLocation();
  // The Pairing page's section already shows the state in full.
  if (pathname === pairingPath || pathname.startsWith(`${pairingPath}/`)) return false;
  return state?.state === "unreachable" || state?.state === "revoked";
}

/** The shell's notice while the peer cannot be reached; it has no close button and leaves on its own. */
export default function PeerNotice() {
  const state = usePeerState();
  const name = t.list.card.remote.name(usePeerName());
  const owner = useAudience() === "owner";
  if (state === null) return null;
  const { icon: Icon, title, body } = noticeText(state, name, owner);
  return (
    <Alert variant="warning" className="has-data-[slot=alert-action]:pr-3">
      <Icon aria-hidden="true" />
      <div className="flex min-w-0 flex-col gap-x-3 gap-y-2 md:flex-row md:items-center">
        <div className="flex min-w-0 flex-1 flex-col gap-x-2 gap-y-0.5 md:flex-row md:items-baseline">
          <AlertTitle>{title}</AlertTitle>
          <AlertDescription>{body}</AlertDescription>
        </div>
        {owner && (
          <AlertAction className="static shrink-0">
            <Button asChild variant="outline" size="sm">
              <Link to={pairingPath}>{t.shell.peer.openPairing}</Link>
            </Button>
          </AlertAction>
        )}
      </div>
    </Alert>
  );
}
