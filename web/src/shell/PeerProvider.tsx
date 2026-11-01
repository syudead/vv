import { createContext, type ReactNode, useContext, useEffect, useState } from "react";
import { useLocation } from "react-router";

import { getPeer, type PeerState } from "../api/peer";
import { subscribeServerEvents } from "../api/serverEvents";
import { useAudience } from "../auth/audience";

/** Each link's host name by slot, as last seen; a slot keeps its name after its link goes. */
type LinkNames = ReadonlyMap<number, string>;

interface PeerView {
  /** Null until the first read or event answers. */
  state: PeerState | null;
  /** Guests never see a name, so their map stays empty. */
  names: LinkNames;
}

const noNames: LinkNames = new Map();

const PeerContext = createContext<PeerView>({ state: null, names: noNames });

function withNames(names: LinkNames, state: PeerState): LinkNames {
  if (!state.links.some((link) => link.name !== undefined)) return names;
  const next = new Map(names);
  for (const link of state.links) {
    if (link.name !== undefined) next.set(link.slot, link.name);
  }
  return next;
}

/**
 * Reads GET /api/peer and, for owners, follows the peer event. Subscribe before reading so no
 * change is missed; an event that arrives while the read is in flight is newer than its answer.
 * Guests open no event stream for it, so they read the state again on each page instead.
 */
export function PeerProvider({ children }: { children: ReactNode }) {
  const owner = useAudience() === "owner";
  const { pathname } = useLocation();
  const readKey = owner ? "" : pathname;
  const [view, setView] = useState<PeerView>({ state: null, names: noNames });
  useEffect(() => {
    let changed = false;
    const apply = (state: PeerState) =>
      setView((current) => ({ state, names: withNames(current.names, state) }));
    const unsubscribe = owner
      ? subscribeServerEvents({
          peer: (state) => {
            changed = true;
            apply(state);
          },
        })
      : undefined;
    const controller = new AbortController();
    getPeer(controller.signal).then(
      (state) => {
        if (!changed) apply(state);
      },
      () => undefined,
    );
    return () => {
      unsubscribe?.();
      controller.abort();
    };
  }, [owner, readKey]);
  return <PeerContext.Provider value={view}>{children}</PeerContext.Provider>;
}

export function usePeerState(): PeerState | null {
  return useContext(PeerContext).state;
}

/**
 * The host name of the link in slot, as `remoteSlot(id)` gives it for an item's id, as last seen; undefined for
 * guests, before the first answer and for slot 0, this node's own items.
 */
export function usePeerName(slot: number): string | undefined {
  return useContext(PeerContext).names.get(slot);
}

/** Test and showcase wrapper with a fixed state. */
export function StaticPeer({
  state,
  children,
}: {
  state: PeerState;
  children: ReactNode;
}) {
  return (
    <PeerContext.Provider value={{ state, names: withNames(noNames, state) }}>
      {children}
    </PeerContext.Provider>
  );
}
