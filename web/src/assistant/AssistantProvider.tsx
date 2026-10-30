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
import { useNavigate } from "react-router";

import {
  type AssistantCandidate,
  type AssistantProposal,
  type AssistantReadinessReason,
  type AssistantTurn,
  getAssistantReadiness,
} from "../api/assistant";
import type { AssistantScreen } from "./screenContext";
import { type Exchange, isWaiting, waitsOnOwner } from "./thread";
import { type Cards, useProposals } from "./useProposals";
import { type ReviewRuns, useReviewRuns } from "./useReviewRuns";
import { type Publish, useThread } from "./useThread";

export type Readiness =
  | { state: "checking" }
  | { state: "ready"; vision: boolean }
  | { state: "notReady"; reason: AssistantReadinessReason }
  | { state: "failed"; error: unknown };

export interface AssistantStore {
  open: boolean;
  exchanges: readonly Exchange[];
  /** The thread is being rebuilt from the server's active turns. */
  loading: boolean;
  waiting: boolean;
  /** Confirmation cards by the exchange whose reply they are. */
  cards: Cards;
  /** Runs read when the panel opened that the thread does not show as cards. */
  runs: readonly AssistantProposal[];
  /** The review runs the thread's review rows follow, by run id. */
  reviews: ReviewRuns;
  /** A turn runs, a run is approved or running, or a review run is judging. */
  working: boolean;
  /** A turn ended on a card or a question while the panel was closed, and the panel has not opened since. */
  awaiting: boolean;
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
  execute: (exchangeId: number, steps: readonly number[]) => void;
  dismiss: (exchangeId: number) => void;
  setDraft: (draft: string) => void;
  checkReadiness: () => void;
}

const AssistantContext = createContext<AssistantStore | null>(null);

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

/** Below lg the panel covers the page a directive just changed, so it closes after one. */
function isWide(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(wideQuery).matches;
}

function useReadiness() {
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
            ? { state: "ready", vision: result.vision }
            : { state: "notReady", reason: result.reason ?? "generation_model_unset" },
        );
      },
      (error: unknown) => {
        if (!controller.signal.aborted) setReadiness({ state: "failed", error });
      },
    );
  }, []);
  const onNotReady = useCallback((reason: AssistantReadinessReason) => {
    setReadiness({ state: "notReady", reason });
  }, []);
  useEffect(() => () => readinessRead.current?.abort(), []);
  return { readiness, checkReadiness, onNotReady };
}

/** The review runs the thread's turns started, in thread order. */
function reviewRunIds(exchanges: readonly Exchange[]): number[] {
  const ids = exchanges.flatMap((exchange) =>
    (exchange.turn?.progress ?? []).flatMap((item) =>
      item.reviewRunId === undefined ? [] : [item.reviewRunId],
    ),
  );
  return [...new Set(ids)];
}

export function AssistantProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const latestOpen = useRef(open);
  latestOpen.current = open;
  const [awaiting, setAwaiting] = useState(false);
  const focusRequested = useRef(false);
  const takeFocus = useCallback(() => {
    const requested = focusRequested.current;
    focusRequested.current = false;
    return requested;
  }, []);
  const [draft, setDraft] = useState("");
  const { readiness, checkReadiness, onNotReady } = useReadiness();

  const onApplied = useCallback(
    (path: string) => {
      void navigate(path);
      if (!isWide()) setOpen(false);
    },
    [navigate],
  );
  const proposals = useProposals(open);
  const settleCard = proposals.settle;
  const onTurn = useCallback(
    (exchangeId: number, turn: AssistantTurn, rebuilt: boolean) => {
      settleCard(exchangeId, turn, rebuilt);
      if (!rebuilt && !latestOpen.current && waitsOnOwner(turn)) setAwaiting(true);
    },
    [settleCard],
  );
  const thread = useThread({ onApplied, onNotReady, onTurn });
  const { screen, publish, exchanges, loading, send, choose, load } = thread;
  const reviewIds = useMemo(() => reviewRunIds(exchanges), [exchanges]);
  const { reviews, reviewing } = useReviewRuns(reviewIds);
  const { cards, runs, running, execute, dismiss, loadRuns } = proposals;

  const toggle = useCallback(
    (byPress: boolean) => {
      const opening = !latestOpen.current;
      if (opening) {
        checkReadiness();
        loadRuns();
        load();
        setAwaiting(false);
        focusRequested.current = byPress;
      }
      setOpen(opening);
    },
    [checkReadiness, loadRuns, load],
  );
  const close = useCallback(() => setOpen(false), []);

  const clearThread = thread.clear;
  const clearCards = proposals.clear;
  const clear = useCallback(() => {
    clearThread();
    clearCards();
  }, [clearThread, clearCards]);
  const waiting = isWaiting(exchanges);
  const store = useMemo<AssistantStore>(
    () => ({
      open,
      exchanges,
      loading,
      waiting,
      cards,
      runs,
      reviews,
      working: waiting || running || reviewing,
      awaiting,
      readiness,
      draft,
      screen,
      takeFocus,
      toggle,
      close,
      send: (text) => send(text),
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
      loading,
      waiting,
      cards,
      runs,
      reviews,
      running,
      reviewing,
      awaiting,
      readiness,
      draft,
      screen,
      takeFocus,
      toggle,
      close,
      send,
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
