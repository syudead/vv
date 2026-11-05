import { useCallback, useEffect, useRef, useState } from "react";

import { getPeer, type PeerLinkState, type PeerState } from "../api/peer";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Skeleton } from "../ui/shadcn/skeleton";

import { PairForm } from "./PairingSectionForm";
import { PEER_LINK_LIMIT } from "./pairing";
import { LinkSection } from "./PairingSectionLink";
import { ReadersSection } from "./PairingSectionReaders";

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

  // The state lines move only on a peer event, so an answer updates the links' details alone.
  const updateLink = useCallback((answer: PeerState) => {
    setView((current) =>
      current.kind === "loaded"
        ? {
            kind: "loaded",
            state: {
              ...current.state,
              links: withDetails(current.state.links, answer.links),
            },
          }
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

  return { view, apply, updateLink, retry: () => void load() };
}

/** links with the details of each one answer holds, keeping the state each link last had. */
function withDetails(links: PeerLinkState[], answer: PeerLinkState[]): PeerLinkState[] {
  return links.map((link) => {
    const answered = answer.find((other) => other.nodeId === link.nodeId);
    if (answered === undefined) return link;
    const { state, reason, fingerprint } = link;
    return { ...answered, state, reason, fingerprint };
  });
}

/** The pairing form while fewer than PEER_LINK_LIMIT links exist, then the limit line in its place. */
function ReadSection({
  links,
  onPaired,
}: {
  links: number;
  onPaired: (state: PeerState) => void;
}) {
  const text = t.settings.pairing.read;
  return (
    <PageSection title={text.heading} description={text.description}>
      {links < PEER_LINK_LIMIT ? (
        <PairForm onPaired={onPaired} />
      ) : (
        <p className="text-muted-foreground">{text.limit}</p>
      )}
    </PageSection>
  );
}

/**
 * The VVMDMs this one reads, one section per link in slot order, then the VVMDM that reads this one.
 * Every section follows GET /api/peer and the peer event.
 */
export default function PairingSection() {
  const text = t.settings.pairing;
  const { view, apply, updateLink, retry } = usePeerState();

  if (view.kind === "loaded") {
    const { state } = view;
    return (
      <>
        <ReadSection links={state.links.length} onPaired={apply} />
        {state.links.map((link) => (
          <LinkSection
            key={link.slot}
            link={link}
            onLinkAnswer={updateLink}
            onUnpaired={() =>
              apply({
                ...state,
                links: state.links.filter((other) => other.nodeId !== link.nodeId),
              })
            }
          />
        ))}
        <ReadersSection
          grant={state.grant}
          onRevoked={() => apply({ ...state, grant: undefined })}
        />
      </>
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
