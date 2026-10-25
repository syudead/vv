import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  type AssistantOutcome,
  type AssistantProposal,
  executeAssistantProposal,
  getAssistantProposal,
  listActiveAssistantProposals,
} from "../api/assistant";
import { subscribeServerEvents } from "../api/serverEvents";
import { isActive, isNewer } from "./proposal";

/** live follows the proposal; the others are what the card collapsed to. */
export type CardView = "live" | "dismissed" | "replaced" | "expired";

export interface Card {
  proposal: AssistantProposal;
  view: CardView;
  executing: boolean;
  /** The last Execute failure other than 404 and 409; the buttons stay. */
  error?: unknown;
}

export type Cards = ReadonlyMap<number, Card>;

export interface Proposals {
  /** Keyed by the exchange whose reply the card is. */
  cards: Cards;
  /** What the panel read when it opened, without the runs the thread shows as cards. */
  runs: readonly AssistantProposal[];
  /** A run is approved or running, so the trigger shows its dot while the panel is closed. */
  running: boolean;
  settle: (exchangeId: number, outcome: AssistantOutcome) => void;
  execute: (exchangeId: number) => void;
  dismiss: (exchangeId: number) => void;
  clear: () => void;
  loadRuns: () => void;
}

function withCard(cards: Cards, exchangeId: number, change: (card: Card) => Card): Cards {
  const card = cards.get(exchangeId);
  if (card === undefined) return cards;
  const next = new Map(cards);
  next.set(exchangeId, change(card));
  return next;
}

/** Applies a read of a proposal to every card that shows it, unless it answers out of order. */
function withProposal(cards: Cards, proposal: AssistantProposal): Cards {
  let next: Map<number, Card> | undefined;
  for (const [id, card] of cards) {
    if (card.proposal.id !== proposal.id || !isNewer(proposal, card.proposal)) continue;
    next ??= new Map(cards);
    next.set(id, { ...card, proposal });
  }
  return next ?? cards;
}

/** Any later outcome means the server discarded the waiting proposal it was sent as previous. */
function replaceWaiting(cards: Cards): Cards {
  let next: Map<number, Card> | undefined;
  for (const [id, card] of cards) {
    if (card.view !== "live" || card.proposal.status !== "proposed") continue;
    next ??= new Map(cards);
    next.set(id, { ...card, view: "replaced" });
  }
  return next ?? cards;
}

function hasActive(proposals: Iterable<AssistantProposal>): boolean {
  for (const proposal of proposals) if (isActive(proposal.status)) return true;
  return false;
}

export function useProposals(open: boolean): Proposals {
  const [cards, setCards] = useState<Cards>(new Map());
  const latestCards = useRef(cards);
  latestCards.current = cards;
  const [listed, setListed] = useState<readonly AssistantProposal[]>([]);
  const executing = useRef(new Set<number>());
  const runsRead = useRef<AbortController | null>(null);

  const loadRuns = useCallback(() => {
    runsRead.current?.abort();
    const controller = new AbortController();
    runsRead.current = controller;
    listActiveAssistantProposals(controller.signal).then(
      (items) => {
        if (!controller.signal.aborted) setListed(items);
      },
      () => undefined,
    );
  }, []);

  const settle = useCallback((exchangeId: number, outcome: AssistantOutcome) => {
    setCards((current) => {
      const next = new Map(replaceWaiting(current));
      if (outcome.kind === "proposal" && outcome.proposal !== undefined) {
        next.set(exchangeId, {
          proposal: outcome.proposal,
          view: "live",
          executing: false,
        });
      }
      return next;
    });
  }, []);

  const execute = useCallback((exchangeId: number) => {
    const card = latestCards.current.get(exchangeId);
    if (card === undefined || card.view !== "live" || card.proposal.status !== "proposed")
      return;
    // A second press before the answer sends nothing; the button is aria-disabled meanwhile.
    if (executing.current.has(exchangeId)) return;
    executing.current.add(exchangeId);
    setCards((current) =>
      withCard(current, exchangeId, (item) => ({
        ...item,
        executing: true,
        error: undefined,
      })),
    );
    const done = (change: (item: Card) => Card) => {
      executing.current.delete(exchangeId);
      setCards((current) =>
        withCard(current, exchangeId, (item) => ({ ...change(item), executing: false })),
      );
    };
    executeAssistantProposal(card.proposal.id).then(
      (result) => {
        if (result.kind === "expired") {
          done((item) => ({ ...item, view: "expired" }));
          return;
        }
        done((item) => ({ ...item, proposal: result.proposal }));
      },
      (error: unknown) => done((item) => ({ ...item, error })),
    );
  }, []);

  const dismiss = useCallback((exchangeId: number) => {
    setCards((current) =>
      withCard(current, exchangeId, (item) =>
        item.view === "live" && item.proposal.status === "proposed" && !item.executing
          ? { ...item, view: "dismissed" }
          : item,
      ),
    );
  }, []);

  const clear = useCallback(() => {
    executing.current.clear();
    setCards(new Map());
  }, []);

  const refreshCards = useCallback(() => {
    const ids = new Set<number>();
    for (const card of latestCards.current.values()) {
      if (isActive(card.proposal.status)) ids.add(card.proposal.id);
    }
    for (const id of ids) {
      getAssistantProposal(id).then(
        (proposal) => setCards((current) => withProposal(current, proposal)),
        () => undefined,
      );
    }
  }, []);

  const cardProposals = useMemo(
    () => [...cards.values()].map((card) => card.proposal),
    [cards],
  );
  const running = hasActive(cardProposals) || hasActive(listed);
  const watching = open || running;

  useEffect(() => {
    if (!watching) return;
    const refresh = () => {
      refreshCards();
      loadRuns();
    };
    return subscribeServerEvents({
      assistant: refresh,
      open: (reconnected) => {
        if (reconnected) refresh();
      },
    });
  }, [watching, refreshCards, loadRuns]);

  useEffect(() => () => runsRead.current?.abort(), []);

  const runs = useMemo(() => {
    const shown = new Set(cardProposals.map((proposal) => proposal.id));
    return listed.filter((proposal) => !shown.has(proposal.id));
  }, [listed, cardProposals]);

  return { cards, runs, running, settle, execute, dismiss, clear, loadRuns };
}
