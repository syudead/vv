import type {
  AssistantOutcome,
  AssistantPin,
  AssistantReadinessReason,
} from "../api/assistant";
import { RequestFailed } from "../api/client";

export type UnavailableReason = "connection" | "model" | "timeout";

/** The reply under one request; an outcome keeps the URL it applied, for the link back. */
export type Reply =
  | { kind: "outcome"; outcome: AssistantOutcome; path?: string }
  | { kind: "unavailable"; reason: UnavailableReason }
  | { kind: "tooManyVideos" }
  | { kind: "failed"; error: unknown };

/** One request and its reply; reply is absent while the turn runs. */
export interface Exchange {
  id: number;
  /** The text the request row shows: the owner's words, or the candidate they chose. */
  shown: string;
  /** The text sent; a clarification choice resends the request it answers. */
  text: string;
  pinned: readonly AssistantPin[];
  /** Whether the screen had a selection when the request was sent. */
  hadSelection: boolean;
  startedAt: number;
  reply?: Reply;
}

/** The outcome a follow-up sends as previous: the last reply that is one. */
export function previousOutcome(
  exchanges: readonly Exchange[],
): AssistantOutcome | undefined {
  for (let index = exchanges.length - 1; index >= 0; index -= 1) {
    const reply = exchanges[index]?.reply;
    if (reply?.kind === "outcome") return reply.outcome;
  }
  return undefined;
}

export function isWaiting(exchanges: readonly Exchange[]): boolean {
  const last = exchanges[exchanges.length - 1];
  return last !== undefined && last.reply === undefined;
}

const readinessReasons: readonly AssistantReadinessReason[] = [
  "ollama_not_saved",
  "generation_model_unset",
];

const unavailableReasons: readonly UnavailableReason[] = [
  "connection",
  "model",
  "timeout",
];

/** A failed turn is a reply, or makes the panel not ready (409), in which case nothing is shown under it. */
export type TurnFailure =
  | { kind: "reply"; reply: Reply }
  | { kind: "notReady"; reason: AssistantReadinessReason };

export function turnFailure(error: unknown): TurnFailure {
  if (!(error instanceof RequestFailed))
    return { kind: "reply", reply: { kind: "failed", error } };
  if (error.code === "assistant_not_ready") {
    const reason = readinessReasons.find((known) => known === error.reason);
    return { kind: "notReady", reason: reason ?? "generation_model_unset" };
  }
  if (error.code === "assistant_unavailable") {
    const reason = unavailableReasons.find((known) => known === error.reason);
    return {
      kind: "reply",
      reply: { kind: "unavailable", reason: reason ?? "connection" },
    };
  }
  if (error.reason === "too_many_videos") {
    return { kind: "reply", reply: { kind: "tooManyVideos" } };
  }
  return { kind: "reply", reply: { kind: "failed", error } };
}
