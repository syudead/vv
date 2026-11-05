import type { ReactNode } from "react";

import { t } from "../i18n";
import { isRemoteId, remoteSlot } from "../lib/remoteId";
import { usePeerName } from "../shell/PeerProvider";
import RemoteMark from "../ui/RemoteMark";

/** The host name of the link that holds the entity itemId names, undefined for guests. */
export function useItemPeerName(itemId: number): string | undefined {
  return usePeerName(remoteSlot(itemId));
}

/** RemoteMark naming the link that holds the item itemId names, for an item whose id is remote. */
export function ItemRemoteMark({
  itemId,
  id,
  size,
}: {
  itemId: number;
  id?: string;
  size?: "sm" | "md";
}) {
  const name = useItemPeerName(itemId);
  return <RemoteMark label={t.list.card.remote.on(name)} id={id} size={size} />;
}

/** A collection page's count line; a remote collection's starts with the mark and its link's name. */
export function RemoteCountLine({
  itemId,
  children,
}: {
  /** The collection's id. */
  itemId: number;
  /** The count; a folder page has none, so its line holds the mark and the name alone. */
  children?: ReactNode;
}) {
  const name = useItemPeerName(itemId);
  if (!isRemoteId(itemId)) return children;
  return (
    <span className="inline-flex items-center gap-1">
      <ItemRemoteMark itemId={itemId} />
      <span aria-hidden="true">
        {t.list.card.remote.name(name)}
        {children !== undefined && " · "}
      </span>
      {children}
    </span>
  );
}
