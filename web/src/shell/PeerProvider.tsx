import { createContext, type ReactNode, useContext, useEffect, useState } from "react";
import { useLocation } from "react-router";

import { getPeer, type PeerState } from "../api/peer";
import { subscribeServerEvents } from "../api/serverEvents";
import { useAudience } from "../auth/audience";

interface PeerView {
  /** Null until the first read or event answers. */
  state: PeerState | null;
  /** The peer's host name as last seen; guests never see it. */
  name: string | undefined;
}

const PeerContext = createContext<PeerView>({ state: null, name: undefined });

/**
 * Reads GET /api/peer and, for owners, follows the peer event. Subscribe before reading so no
 * change is missed; an event that arrives while the read is in flight is newer than its answer.
 * Guests open no event stream for it, so they read the state again on each page instead.
 */
export function PeerProvider({ children }: { children: ReactNode }) {
  const owner = useAudience() === "owner";
  const { pathname } = useLocation();
  const readKey = owner ? "" : pathname;
  const [view, setView] = useState<PeerView>({ state: null, name: undefined });
  useEffect(() => {
    let changed = false;
    const apply = (state: PeerState) =>
      setView((current) => ({ state, name: state.link?.name ?? current.name }));
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

/** The peer's host name as last seen; undefined for guests and before the first answer. */
export function usePeerName(): string | undefined {
  return useContext(PeerContext).name;
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
    <PeerContext.Provider value={{ state, name: state.link?.name }}>
      {children}
    </PeerContext.Provider>
  );
}
