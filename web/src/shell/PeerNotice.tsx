import { type LucideIcon, ShieldAlert, Unlink, Unplug } from "lucide-react";
import { Link, useLocation } from "react-router";

import type { PeerLinkState, PeerState } from "../api/peer";
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

/** The links whose items are hidden: every one that is not connected. */
function linksDown(state: PeerState | null): PeerLinkState[] {
  return state?.links.filter((link) => link.state !== "connected") ?? [];
}

function noticeText(down: PeerLinkState[], name: string, owner: boolean): NoticeText {
  const words = t.shell.peer;
  const link = down[0];
  if (!owner) return { icon: Unplug, title: words.guestTitle, body: words.guestBody };
  if (link === undefined || down.length > 1)
    return {
      icon: Unplug,
      title: words.severalTitle(down.length),
      body: words.severalBody,
    };
  if (link.state === "revoked")
    return { icon: Unlink, title: words.revokedTitle(name), body: words.revokedBody };
  if (link.reason === "certificate")
    return {
      icon: ShieldAlert,
      title: words.certificateTitle(name),
      body: words.certificateBody,
    };
  return { icon: Unplug, title: words.networkTitle(name), body: words.networkBody };
}

/** Whether the shell draws the notice: a link exists but its items are hidden. */
export function usePeerNoticeShown(): boolean {
  const state = usePeerState();
  const { pathname } = useLocation();
  // The Pairing page's section already shows the state in full.
  if (pathname === pairingPath || pathname.startsWith(`${pairingPath}/`)) return false;
  return linksDown(state).length > 0;
}

/** The shell's notice while a link cannot be reached; it has no close button and leaves on its own. */
export default function PeerNotice() {
  const down = linksDown(usePeerState());
  const name = t.list.card.remote.name(usePeerName(down[0]?.slot ?? 0));
  const owner = useAudience() === "owner";
  if (down.length === 0) return null;
  const { icon: Icon, title, body } = noticeText(down, name, owner);
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
