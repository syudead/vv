import { Link, Unlink, Unplug } from "lucide-react";

import type { PeerLinkState } from "../api/peer";
import { t } from "../i18n";
import { Badge } from "../ui/shadcn/badge";

const looks = {
  connected: { icon: Link, variant: "success" },
  unreachable: { icon: Unplug, variant: "warning" },
  revoked: { icon: Unlink, variant: "warning" },
} as const;

/** One link's state in a section header; the reason stays in the section's detail line. */
export function PeerLinkBadge({ state }: { state: PeerLinkState["state"] }) {
  const { icon: Icon, variant } = looks[state];
  return (
    <Badge variant={variant}>
      <Icon aria-hidden="true" data-icon="inline-start" />
      {t.settings.pairing.badge[state]}
    </Badge>
  );
}
