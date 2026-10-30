import { type AssistantTurn, getAssistantTurn } from "../api/assistant";
import { subscribeServerEvents } from "../api/serverEvents";

/**
 * Resolves with the turn once it is no longer running. The server sends an assistant event on
 * every change to a turn, so the turn is read again on each one, on a reconnect, and once at the
 * start for an event sent before the subscription. A failed read rejects; an abort settles nothing.
 */
export function followTurn(
  turn: AssistantTurn,
  signal: AbortSignal,
): Promise<AssistantTurn> {
  if (turn.status !== "running") return Promise.resolve(turn);
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (settle: () => void) => {
      if (settled) return;
      settled = true;
      unsubscribe();
      signal.removeEventListener("abort", onAbort);
      settle();
    };
    const onAbort = () => finish(() => undefined);
    const read = () => {
      getAssistantTurn(turn.id, signal).then(
        (current) => {
          if (current.status !== "running") finish(() => resolve(current));
        },
        (error: unknown) => {
          if (!signal.aborted) finish(() => reject(error));
        },
      );
    };
    const unsubscribe = subscribeServerEvents({
      assistant: read,
      open: (reconnected) => {
        if (reconnected) read();
      },
    });
    signal.addEventListener("abort", onAbort);
    if (signal.aborted) onAbort();
    else read();
  });
}
