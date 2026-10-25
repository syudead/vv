import type { ReactNode } from "react";

import { t } from "../i18n";
import { usePeerName } from "../shell/PeerProvider";
import RemoteMark from "../ui/RemoteMark";

/** RemoteMark naming the peer, for an item whose id is remote. */
export function ItemRemoteMark({ id, size }: { id?: string; size?: "sm" | "md" }) {
  const name = usePeerName();
  return <RemoteMark label={t.list.card.remote.on(name)} id={id} size={size} />;
}

/** A collection page's count line; a remote collection's starts with the mark and the peer's name. */
export function RemoteCountLine({
  remote,
  children,
}: {
  remote: boolean;
  /** The count; a folder page has none, so its line holds the mark and the name alone. */
  children?: ReactNode;
}) {
  const name = usePeerName();
  if (!remote) return children;
  return (
    <span className="inline-flex items-center gap-1">
      <ItemRemoteMark />
      <span aria-hidden="true">
        {t.list.card.remote.name(name)}
        {children !== undefined && " · "}
      </span>
      {children}
    </span>
  );
}
