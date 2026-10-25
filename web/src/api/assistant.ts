import { request } from "./client";
import type { components } from "./gen/openapi";

// The only caller of the /api/assistant operations; every one is owner-only.

export type AssistantReadiness = components["schemas"]["AssistantReadiness"];
export type AssistantReadinessReason = NonNullable<AssistantReadiness["reason"]>;
export type AssistantTurnRequest = components["schemas"]["AssistantTurnRequest"];
export type AssistantTurnContext = components["schemas"]["AssistantTurnContext"];
export type AssistantRoute = components["schemas"]["AssistantRoute"];
export type AssistantPin = components["schemas"]["AssistantPin"];
export type AssistantOutcome = components["schemas"]["AssistantTurnOutcome"];
export type AssistantCriteria = components["schemas"]["AssistantCriteria"];
export type AssistantDestination = components["schemas"]["AssistantDestination"];
export type AssistantCandidate = components["schemas"]["AssistantCandidate"];
export type AssistantLabel = components["schemas"]["AssistantLabel"];
export type AssistantLabelKind = components["schemas"]["AssistantLabelKind"];
export type AssistantProposal = components["schemas"]["AssistantProposal"];

/** Sends no request to Ollama; the panel reads it each time it opens. */
export function getAssistantReadiness(signal?: AbortSignal): Promise<AssistantReadiness> {
  return request<AssistantReadiness>("/api/assistant/readiness", { signal });
}

/**
 * One request and its outcome. 400 too_many_videos, 409 assistant_not_ready and 503
 * assistant_unavailable reject with RequestFailed; nothing is stored then.
 */
export async function createAssistantTurn(
  body: AssistantTurnRequest,
  signal?: AbortSignal,
): Promise<AssistantOutcome> {
  const response = await request<components["schemas"]["AssistantTurnResponse"]>(
    "/api/assistant/turns",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal,
    },
  );
  return response.outcome;
}
