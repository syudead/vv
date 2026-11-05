import type {
  AssistantPin,
  AssistantReadinessReason,
  AssistantTurn,
} from "../api/assistant";
import { RequestFailed } from "../api/client";

/** A reply the panel draws when no turn answers the request, or no longer can. */
export type LocalReply =
  | { kind: "tooManyVideos" }
  | { kind: "busy" }
  | { kind: "expired" }
  | { kind: "failed"; error: unknown };

/** One request and what answers it: the server's turn, or a local reply. */
export interface Exchange {
  id: number;
  /** The text the request row shows: the owner's words, or the candidate they chose. */
  shown: string;
  /** The text sent; a clarification choice resends the request it answers. */
  text: string;
  pinned: readonly AssistantPin[];
  /** Whether the screen had a selection when the request was sent. */
  hadSelection: boolean;
  /** Epoch ms; orders the thread. */
  startedAt: number;
  /** The pathname this tab sent the request from; absent for a turn rebuilt from the server. */
  sentFrom?: string;
  /** The latest read of the turn; absent while the request is being sent. */
  turn?: AssistantTurn;
  /** The URL the turn's directive applied in this tab. */
  applied?: string;
  reply?: LocalReply;
}

/** The turn a follow-up continues: the last one in the thread. */
export function previousTurnId(exchanges: readonly Exchange[]): number | undefined {
  for (let index = exchanges.length - 1; index >= 0; index -= 1) {
    const turn = exchanges[index]?.turn;
    if (turn !== undefined) return turn.id;
  }
  return undefined;
}

/** Sending, or the turn still runs. */
export function isPending(exchange: Exchange): boolean {
  if (exchange.reply !== undefined) return false;
  return exchange.turn === undefined || exchange.turn.status === "running";
}

export function isWaiting(exchanges: readonly Exchange[]): boolean {
  return exchanges.some(isPending);
}

/** A turn that ended on a question to the owner, or on a card for them to approve. */
export function waitsOnOwner(turn: AssistantTurn): boolean {
  if (turn.status === "clarify") return true;
  return turn.proposal?.status === "proposed";
}

/** A read answered out of order never takes a finished turn back, nor drops progress items. */
export function isNewerTurn(next: AssistantTurn, current: AssistantTurn): boolean {
  if (current.status !== "running") return next.status !== "running";
  return next.status !== "running" || next.progress.length >= current.progress.length;
}

/** The exchange a turn read from the server starts. */
export function rebuiltExchange(id: number, turn: AssistantTurn): Exchange {
  return {
    id,
    shown: turn.request,
    text: turn.request,
    pinned: [],
    hadSelection: false,
    startedAt: Date.parse(turn.startedAt),
    turn,
  };
}

export function byStart(exchanges: readonly Exchange[]): Exchange[] {
  return [...exchanges].sort((a, b) => a.startedAt - b.startedAt);
}

const readinessReasons: readonly AssistantReadinessReason[] = [
  "ollama_not_saved",
  "generation_model_unset",
  "generation_model_without_tools",
];

/** A failed request is a reply, or makes the panel not ready (409), in which case nothing is shown under it. */
export type TurnFailure =
  | { kind: "reply"; reply: LocalReply }
  | { kind: "notReady"; reason: AssistantReadinessReason };

export function turnFailure(error: unknown): TurnFailure {
  if (!(error instanceof RequestFailed))
    return { kind: "reply", reply: { kind: "failed", error } };
  if (error.code === "assistant_not_ready") {
    const reason = readinessReasons.find((known) => known === error.reason);
    return { kind: "notReady", reason: reason ?? "generation_model_unset" };
  }
  if (error.reason === "too_many_videos") {
    return { kind: "reply", reply: { kind: "tooManyVideos" } };
  }
  return { kind: "reply", reply: { kind: "failed", error } };
}
