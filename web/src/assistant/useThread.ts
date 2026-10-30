import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";

import {
  type AssistantCandidate,
  type AssistantPin,
  type AssistantReadinessReason,
  type AssistantTurn,
  createAssistantTurn,
  getAssistantTurn,
  listActiveAssistantTurns,
} from "../api/assistant";
import { RequestFailed } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import {
  readThreadClearedAt,
  writeThreadClearedAt,
} from "../preferences/assistantThread";
import { directivePath } from "./links";
import { type AssistantScreen, hasSelection, turnContext } from "./screenContext";
import {
  byStart,
  type Exchange,
  isNewerTurn,
  isWaiting,
  previousTurnId,
  rebuiltExchange,
  turnFailure,
} from "./thread";

export type Publish = (screen: AssistantScreen) => () => void;

export interface ThreadEvents {
  /** A directive applied: the URL it opens. */
  onApplied: (path: string) => void;
  onNotReady: (reason: AssistantReadinessReason) => void;
  /** A turn ended, or was rebuilt from the server already ended. */
  onTurn: (exchangeId: number, turn: AssistantTurn, rebuilt: boolean) => void;
}

interface SendOptions {
  shown?: string;
  pinned?: readonly AssistantPin[];
}

/**
 * The thread of exchanges, which follows each running turn through assistant events and is rebuilt
 * from the server's active turns when the panel opens.
 */
export function useThread({ onApplied, onNotReady, onTurn }: ThreadEvents) {
  const location = useLocation();
  const latestLocation = useRef(location);
  latestLocation.current = location;
  const [screen, setScreen] = useState<AssistantScreen>({});
  const latestScreen = useRef(screen);
  latestScreen.current = screen;
  const [exchanges, setExchanges] = useState<readonly Exchange[]>([]);
  const latestExchanges = useRef(exchanges);
  latestExchanges.current = exchanges;
  const [loading, setLoading] = useState(false);
  const nextId = useRef(1);
  /** The exchange showing each turn, kept at once so two reads never add one turn twice. */
  const shownTurns = useRef(new Map<number, number>());
  /** Turns whose end was handled, so a directive applies and a card appears once. */
  const ended = useRef(new Set<number>());
  const clearedAt = useRef(readThreadClearedAt());
  const sending = useRef<AbortController | null>(null);
  const loadRequest = useRef<AbortController | null>(null);

  const publish = useCallback<Publish>((next) => {
    setScreen(next);
    return () => setScreen((current) => (current === next ? {} : current));
  }, []);

  const update = useCallback((id: number, change: (exchange: Exchange) => Exchange) => {
    setExchanges((current) =>
      current.map((exchange) => (exchange.id === id ? change(exchange) : exchange)),
    );
  }, []);

  /**
   * Shows a read of the turn, and handles its end once. A directive applies only in the tab that
   * sent the request, while it is still on the page it was sent from.
   */
  const receive = useCallback(
    (exchangeId: number, turn: AssistantTurn, sentFrom: string | undefined) => {
      shownTurns.current.set(turn.id, exchangeId);
      const ending = turn.status !== "running" && !ended.current.has(turn.id);
      if (ending) ended.current.add(turn.id);
      const stayed = sentFrom !== undefined && latestLocation.current.pathname === sentFrom;
      const path = ending && stayed ? directivePath(turn.directive) : undefined;
      update(exchangeId, (exchange) =>
        exchange.turn !== undefined && !isNewerTurn(turn, exchange.turn)
          ? exchange
          : { ...exchange, turn, ...(path !== undefined && { applied: path }) },
      );
      if (!ending) return;
      onTurn(exchangeId, turn, false);
      if (path !== undefined) onApplied(path);
    },
    [onApplied, onTurn, update],
  );

  /** Adds the turns the thread does not show yet, in the order they started. */
  const merge = useCallback(
    (turns: readonly AssistantTurn[]) => {
      const added: Exchange[] = [];
      for (const turn of turns) {
        const shown = shownTurns.current.get(turn.id);
        if (shown !== undefined) {
          const exchange = latestExchanges.current.find((item) => item.id === shown);
          receive(shown, turn, exchange?.sentFrom);
          continue;
        }
        const exchange = rebuiltExchange(nextId.current++, turn);
        shownTurns.current.set(turn.id, exchange.id);
        // A turn that ended before the thread showed it applies nothing here.
        if (turn.status !== "running") ended.current.add(turn.id);
        added.push(exchange);
      }
      if (added.length === 0) return;
      setExchanges((current) => byStart([...current, ...added]));
      for (const exchange of added) {
        if (exchange.turn?.status !== "running" && exchange.turn !== undefined) {
          onTurn(exchange.id, exchange.turn, true);
        }
      }
    },
    [onTurn, receive],
  );

  /** Reads the active turns: the panel opened, perhaps after a reload. */
  const load = useCallback(() => {
    loadRequest.current?.abort();
    const controller = new AbortController();
    loadRequest.current = controller;
    setLoading(true);
    listActiveAssistantTurns(controller.signal).then(
      (turns) => {
        if (controller.signal.aborted) return;
        const kept = turns.filter((turn) => Date.parse(turn.startedAt) >= clearedAt.current);
        merge(kept.reverse());
        setLoading(false);
      },
      () => {
        if (!controller.signal.aborted) setLoading(false);
      },
    );
  }, [merge]);

  /** The running turn a 409 named joins the thread, whenever it started. */
  const adopt = useCallback(
    (turnId: number) => {
      getAssistantTurn(turnId).then(
        (turn) => merge([turn]),
        () => undefined,
      );
    },
    [merge],
  );

  const send = useCallback(
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
        sentFrom: latestLocation.current.pathname,
      };
      const previous = previousTurnId(current);
      const context = turnContext(latestLocation.current, latestScreen.current, pinned);
      setExchanges([...current, exchange]);
      const controller = new AbortController();
      sending.current = controller;
      createAssistantTurn(
        { text, context, ...(previous !== undefined && { previousTurnId: previous }) },
        controller.signal,
      ).then(
        (result) => {
          if (controller.signal.aborted) return;
          if (result.kind === "busy") {
            update(exchange.id, (item) => ({ ...item, reply: { kind: "busy" } }));
            adopt(result.turnId);
            return;
          }
          receive(exchange.id, result.turn, exchange.sentFrom);
        },
        (error: unknown) => {
          if (controller.signal.aborted) return;
          const failure = turnFailure(error);
          if (failure.kind === "reply") {
            update(exchange.id, (item) => ({ ...item, reply: failure.reply }));
            return;
          }
          setExchanges((all) => all.filter((item) => item.id !== exchange.id));
          onNotReady(failure.reason);
        },
      );
    },
    [adopt, onNotReady, receive, update],
  );

  const choose = useCallback(
    (exchange: Exchange, candidate: AssistantCandidate) => {
      const pin: AssistantPin = {
        kind: candidate.kind,
        id: candidate.id,
        name: candidate.name,
      };
      send(exchange.text, { shown: candidate.name, pinned: [...exchange.pinned, pin] });
    },
    [send],
  );

  // Clearing stops following the thread's turns, so a late result neither changes the page nor
  // blocks the next request; a reload rebuilds only the turns started after it.
  const clear = useCallback(() => {
    sending.current?.abort();
    sending.current = null;
    loadRequest.current?.abort();
    setLoading(false);
    const now = Date.now();
    clearedAt.current = now;
    writeThreadClearedAt(now);
    shownTurns.current.clear();
    setExchanges([]);
  }, []);

  const runningKey = exchanges
    .flatMap((exchange) =>
      exchange.turn?.status === "running" && exchange.reply === undefined
        ? [exchange.turn.id]
        : [],
    )
    .join(",");

  useEffect(() => {
    if (runningKey === "") return;
    const controller = new AbortController();
    const read = () => {
      for (const exchange of latestExchanges.current) {
        const turn = exchange.turn;
        if (turn?.status !== "running" || exchange.reply !== undefined) continue;
        getAssistantTurn(turn.id, controller.signal).then(
          (current) => receive(exchange.id, current, exchange.sentFrom),
          (error: unknown) => {
            if (controller.signal.aborted) return;
            if (error instanceof RequestFailed && error.status === 404) {
              ended.current.add(turn.id);
              update(exchange.id, (item) => ({ ...item, reply: { kind: "expired" } }));
            }
          },
        );
      }
    };
    const unsubscribe = subscribeServerEvents({
      assistant: read,
      open: (reconnected) => {
        if (reconnected) read();
      },
    });
    // An event sent before this subscription would otherwise be missed.
    read();
    return () => {
      controller.abort();
      unsubscribe();
    };
  }, [runningKey, receive, update]);

  useEffect(
    () => () => {
      sending.current?.abort();
      loadRequest.current?.abort();
    },
    [],
  );

  return { screen, publish, exchanges, loading, send, choose, clear, load };
}
