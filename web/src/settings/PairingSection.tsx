import { useCallback, useEffect, useRef, useState } from "react";

import { getPeer, type PeerState } from "../api/peer";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Skeleton } from "../ui/shadcn/skeleton";

import { PairedSection } from "./PairingSectionPaired";
import { UnpairedSection } from "./PairingSectionUnpaired";

type PeerView =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "loaded"; state: PeerState };

/**
 * Subscribe before reading so no change is missed; a peer event that arrives while the read is in
 * flight is newer than its answer, which is then dropped.
 */
function usePeerState() {
  const [view, setView] = useState<PeerView>({ kind: "loading" });
  const changes = useRef(0);

  const apply = useCallback((state: PeerState) => {
    changes.current += 1;
    setView({ kind: "loaded", state });
  }, []);

  // The state line moves only on a peer event, so an answer updates the peer's details alone.
  const updatePeer = useCallback((answer: PeerState) => {
    setView((current) =>
      current.kind === "loaded" && answer.peer !== undefined
        ? { kind: "loaded", state: { ...current.state, peer: answer.peer } }
        : current,
    );
  }, []);

  const load = useCallback(async (signal?: AbortSignal) => {
    const before = changes.current;
    setView({ kind: "loading" });
    try {
      const state = await getPeer(signal);
      if (changes.current === before) setView({ kind: "loaded", state });
    } catch (failure) {
      if (signal?.aborted || changes.current !== before) return;
      setView({ kind: "failed", reason: errorText(failure) });
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const unsubscribe = subscribeServerEvents({ peer: apply });
    void load(controller.signal);
    return () => {
      unsubscribe();
      controller.abort();
    };
  }, [apply, load]);

  return { view, apply, updatePeer, retry: () => void load() };
}

/** One section whose face follows GET /api/peer and the peer event. */
export default function PairingSection() {
  const text = t.settings.pairing;
  const { view, apply, updatePeer, retry } = usePeerState();

  if (view.kind === "loaded") {
    const { state } = view;
    if (state.state === "unpaired") return <UnpairedSection onPaired={apply} />;
    return (
      <PairedSection
        state={state}
        onPeerAnswer={updatePeer}
        onUnpaired={() => apply({ state: "unpaired", node: state.node })}
      />
    );
  }
  return (
    <PageSection title={t.settings.pages.pairing}>
      {view.kind === "loading" ? (
        <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="h-10" />
          <Skeleton className="h-8 sm:w-sm" />
        </div>
      ) : (
        <div>
          <ErrorState
            title={text.loadFailed(view.reason)}
            retryLabel={t.common.retry}
            onRetry={retry}
          />
        </div>
      )}
    </PageSection>
  );
}
