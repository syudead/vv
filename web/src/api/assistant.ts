import { apiFetch, request, toRequestFailed } from "./client";
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

export type AssistantProposalStatus = AssistantProposal["status"];

/** What Execute led to: approved now, already past proposed (the 409 body), or gone (404). */
export type ExecuteResult =
  | { kind: "approved"; proposal: AssistantProposal }
  | { kind: "notPending"; proposal: AssistantProposal }
  | { kind: "expired" };

/** Other failures reject with RequestFailed; the card keeps its buttons then. */
export async function executeAssistantProposal(id: number): Promise<ExecuteResult> {
  const response = await apiFetch(`/api/assistant/proposals/${String(id)}/execute`, {
    method: "POST",
  });
  if (response.ok) {
    return { kind: "approved", proposal: (await response.json()) as AssistantProposal };
  }
  if (response.status === 404) return { kind: "expired" };
  if (response.status === 409) {
    const body = (await response
      .clone()
      .json()
      .catch(() => null)) as {
      proposal?: AssistantProposal;
    } | null;
    if (body?.proposal !== undefined)
      return { kind: "notPending", proposal: body.proposal };
  }
  throw await toRequestFailed(response);
}

/** The panel re-reads a shown proposal on each assistant event. */
export function getAssistantProposal(
  id: number,
  signal?: AbortSignal,
): Promise<AssistantProposal> {
  return request<AssistantProposal>(`/api/assistant/proposals/${String(id)}`, { signal });
}

/** Approved and running proposals, and those finished within the last hour, newest first. */
export async function listActiveAssistantProposals(
  signal?: AbortSignal,
): Promise<AssistantProposal[]> {
  const list = await request<components["schemas"]["AssistantProposalList"]>(
    "/api/assistant/proposals?active=true",
    { signal },
  );
  return list.items;
}
