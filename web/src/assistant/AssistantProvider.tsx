import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useLocation, useNavigate } from "react-router";

import {
  type AssistantCandidate,
  type AssistantOutcome,
  type AssistantPin,
  type AssistantProposal,
  type AssistantReadinessReason,
  type AssistantTurn,
  createAssistantTurn,
  getAssistantReadiness,
} from "../api/assistant";
import { followTurn } from "./followTurn";
import { criteriaPath, destinationPath } from "./links";
import { type AssistantScreen, hasSelection, turnContext } from "./screenContext";
import {
  type Exchange,
  isWaiting,
  previousTurnId,
  type Reply,
  turnFailure,
  turnOutcome,
} from "./thread";
import { type Cards, useProposals } from "./useProposals";
import { type ReviewRuns, useReviewRuns } from "./useReviewRuns";

export type Readiness =
  | { state: "checking" }
  | { state: "ready" }
  | { state: "notReady"; reason: AssistantReadinessReason }
  | { state: "failed"; error: unknown };

export interface AssistantStore {
  open: boolean;
  exchanges: readonly Exchange[];
  waiting: boolean;
  /** Confirmation cards by the exchange whose reply they are. */
  cards: Cards;
  /** Runs read when the panel opened that the thread does not show as cards. */
  runs: readonly AssistantProposal[];
  /** The review runs the thread's review rows follow, by run id. */
  reviews: ReviewRuns;
  /** A turn waits, a run is approved or running, or a review run is judging. */
  working: boolean;
  readiness: Readiness;
  draft: string;
  screen: AssistantScreen;
  /** True once after the panel opened by a press, so the composer takes the focus then only. */
  takeFocus: () => boolean;
  /** Opens or closes the panel; byPress moves the focus to the field when it opens. */
  toggle: (byPress: boolean) => void;
  close: () => void;
  send: (text: string) => void;
  choose: (exchange: Exchange, candidate: AssistantCandidate) => void;
  clear: () => void;
  execute: (exchangeId: number) => void;
  dismiss: (exchangeId: number) => void;
  setDraft: (draft: string) => void;
  checkReadiness: () => void;
}

const AssistantContext = createContext<AssistantStore | null>(null);

type Publish = (screen: AssistantScreen) => () => void;

const noPublish: Publish = () => () => undefined;
const PublishContext = createContext<Publish>(noPublish);

/** Null outside the provider, such as in page tests that render a page alone. */
export function useAssistant(): AssistantStore | null {
  return useContext(AssistantContext);
}

/** A page tells the assistant its selection and the name it shows while it is mounted. */
export function useAssistantScreen(screen: AssistantScreen): void {
  const publish = useContext(PublishContext);
  const key = JSON.stringify(screen);
  useEffect(() => publish(JSON.parse(key) as AssistantScreen), [publish, key]);
}

const wideQuery = "(min-width: 1024px)";

/** Below lg the panel covers the page an outcome just changed, so it closes after one. */
function isWide(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(wideQuery).matches;
}

/** The URL a filter or navigate outcome applies at once, without confirmation. */
function appliedPath(outcome: AssistantOutcome | undefined): string | undefined {
  if (outcome === undefined) return undefined;
  if (outcome.kind === "filter" && outcome.criteria !== undefined) {
    return criteriaPath(outcome.criteria);
  }
  if (outcome.kind === "navigate" && outcome.destination !== undefined) {
    return destinationPath(outcome.destination);
  }
  return undefined;
}

interface SendOptions {
  shown?: string;
  pinned?: readonly AssistantPin[];
}

function useThread(
  onApplied: (path: string) => void,
  onNotReady: (reason: AssistantReadinessReason) => void,
  onTurn: (exchangeId: number, turn: AssistantTurn) => void,
) {
  const location = useLocation();
  const latestLocation = useRef(location);
  latestLocation.current = location;
  const [screen, setScreen] = useState<AssistantScreen>({});
  const latestScreen = useRef(screen);
  latestScreen.current = screen;
  const [exchanges, setExchanges] = useState<readonly Exchange[]>([]);
  const latestExchanges = useRef(exchanges);
  latestExchanges.current = exchanges;
  const nextId = useRef(1);
  const turnRequest = useRef<AbortController | null>(null);

  const publish = useCallback<Publish>((next) => {
    setScreen(next);
    return () => setScreen((current) => (current === next ? {} : current));
  }, []);

  const settle = useCallback((id: number, reply: Reply) => {
    setExchanges((current) =>
      current.map((exchange) => (exchange.id === id ? { ...exchange, reply } : exchange)),
    );
  }, []);

  const sendTurn = useCallback(
    (text: string, options: SendOptions = {}) => {
      const current = latestExchanges.current;
      if (isWaiting(current)) return;
      const pinned = options.pinned ?? [];
      const exchange: Exchange = {
        id: nextId.current++,
        shown: options.shown ?? text,
        text,
        pinned,
        hadSelection: hasSelection(latestScreen.current),
        startedAt: Date.now(),
      };
      const previous = previousTurnId(current);
      const context = turnContext(latestLocation.current, latestScreen.current, pinned);
      setExchanges([...current, exchange]);
      const controller = new AbortController();
      turnRequest.current = controller;
      createAssistantTurn(
        {
          text,
          context,
          ...(previous !== undefined && { previousTurnId: previous }),
        },
        controller.signal,
      )
        .then((started) => followTurn(started, controller.signal))
        .then(
          (turn) => {
            if (controller.signal.aborted) return;
            const outcome = turnOutcome(turn);
            const path = appliedPath(outcome);
            settle(exchange.id, {
              kind: "turn",
              turn,
              ...(outcome !== undefined && { outcome }),
              ...(path !== undefined && { path }),
            });
            onTurn(exchange.id, turn);
            if (path !== undefined) onApplied(path);
          },
          (error: unknown) => {
            if (controller.signal.aborted) return;
            const failure = turnFailure(error);
            if (failure.kind === "reply") {
              settle(exchange.id, failure.reply);
              return;
            }
            setExchanges((all) => all.filter((item) => item.id !== exchange.id));
            onNotReady(failure.reason);
          },
        );
    },
    [onApplied, onNotReady, onTurn, settle],
  );

  const choose = useCallback(
    (exchange: Exchange, candidate: AssistantCandidate) => {
      const pin: AssistantPin = {
        kind: candidate.kind,
        id: candidate.id,
        name: candidate.name,
      };
      sendTurn(exchange.text, {
        shown: candidate.name,
        pinned: [...exchange.pinned, pin],
      });
    },
    [sendTurn],
  );

  // Clearing stops following a running turn, so its late result neither changes the page nor blocks the next request.
  const clear = useCallback(() => {
    turnRequest.current?.abort();
    turnRequest.current = null;
    setExchanges([]);
  }, []);
  useEffect(() => () => turnRequest.current?.abort(), []);
  return { screen, publish, exchanges, sendTurn, choose, clear };
}

export function AssistantProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const focusRequested = useRef(false);
  const takeFocus = useCallback(() => {
    const requested = focusRequested.current;
    focusRequested.current = false;
    return requested;
  }, []);
  const [draft, setDraft] = useState("");
  const [readiness, setReadiness] = useState<Readiness>({ state: "checking" });
  const readinessRead = useRef<AbortController | null>(null);

  const checkReadiness = useCallback(() => {
    readinessRead.current?.abort();
    const controller = new AbortController();
    readinessRead.current = controller;
    setReadiness((current) =>
      current.state === "ready" ? current : { state: "checking" },
    );
    getAssistantReadiness(controller.signal).then(
      (result) => {
        if (controller.signal.aborted) return;
        setReadiness(
          result.ready
            ? { state: "ready" }
            : { state: "notReady", reason: result.reason ?? "generation_model_unset" },
        );
      },
      (error: unknown) => {
        if (!controller.signal.aborted) setReadiness({ state: "failed", error });
      },
    );
  }, []);

  const onApplied = useCallback(
    (path: string) => {
      void navigate(path);
      if (!isWide()) setOpen(false);
    },
    [navigate],
  );
  const onNotReady = useCallback((reason: AssistantReadinessReason) => {
    setReadiness({ state: "notReady", reason });
  }, []);
  const proposals = useProposals(open);
  const reviewRuns = useReviewRuns();
  const thread = useThread(onApplied, onNotReady, proposals.settle);
  const { loadRuns } = proposals;

  const latestOpen = useRef(open);
  latestOpen.current = open;
  const toggle = useCallback(
    (byPress: boolean) => {
      const opening = !latestOpen.current;
      if (opening) {
        checkReadiness();
        loadRuns();
        focusRequested.current = byPress;
      }
      setOpen(opening);
    },
    [checkReadiness, loadRuns],
  );
  const close = useCallback(() => setOpen(false), []);
  useEffect(() => () => readinessRead.current?.abort(), []);

  const { screen, publish, exchanges, sendTurn, choose } = thread;
  const clearThread = thread.clear;
  const { cards, runs, running, execute, dismiss } = proposals;
  const clearCards = proposals.clear;
  const { reviews, reviewing } = reviewRuns;
  const clearReviews = reviewRuns.clear;
  const clear = useCallback(() => {
    clearThread();
    clearCards();
    clearReviews();
  }, [clearThread, clearCards, clearReviews]);
  const store = useMemo<AssistantStore>(
    () => ({
      open,
      exchanges,
      waiting: isWaiting(exchanges),
      cards,
      runs,
      reviews,
      working: isWaiting(exchanges) || running || reviewing,
      readiness,
      draft,
      screen,
      takeFocus,
      toggle,
      close,
      send: (text) => sendTurn(text),
      choose,
      clear,
      execute,
      dismiss,
      setDraft,
      checkReadiness,
    }),
    [
      open,
      exchanges,
      cards,
      runs,
      reviews,
      running,
      reviewing,
      readiness,
      draft,
      screen,
      takeFocus,
      toggle,
      close,
      sendTurn,
      choose,
      clear,
      execute,
      dismiss,
      checkReadiness,
    ],
  );

  return (
    <PublishContext.Provider value={publish}>
      <AssistantContext.Provider value={store}>{children}</AssistantContext.Provider>
    </PublishContext.Provider>
  );
}
