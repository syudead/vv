import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// The only caller of the /api/assistant operations; every one is owner-only.

export type AssistantReadiness = components["schemas"]["AssistantReadiness"];
export type AssistantReadinessReason = NonNullable<AssistantReadiness["reason"]>;
/**
 * Interim: the server now answers a request as a turn job (202), while the panel still shows the
 * one-shot outcome below. These types keep the panel compiling until it follows turns, which
 * replaces them with AssistantTurn.
 */
export type AssistantTurnRequest = Omit<
  components["schemas"]["AssistantTurnRequest"],
  "previousTurnId"
> & { previous?: AssistantOutcome };
export type AssistantTurnContext = components["schemas"]["AssistantTurnContext"];
export type AssistantRoute = components["schemas"]["AssistantRoute"];
export type AssistantPin = components["schemas"]["AssistantPin"];
export interface AssistantOutcome {
  kind:
    | "filter"
    | "navigate"
    | "answer"
    | "proposal"
    | "review"
    | "clarify"
    | "unsupported"
    | "unreadable";
  criteria?: components["schemas"]["AssistantCriteria"];
  destination?: components["schemas"]["AssistantDestination"];
  question?: "count" | "appearances" | "recent" | "list";
  total?: number;
  items?: { id: number; title: string }[];
  link?: {
    criteria?: components["schemas"]["AssistantCriteria"];
    destination?: components["schemas"]["AssistantDestination"];
  };
  proposal?: components["schemas"]["AssistantProposal"];
  run?: components["schemas"]["AssistantReviewRun"];
  about?: "operation" | "target" | "value";
  candidates?: components["schemas"]["AssistantCandidate"][];
  names?: string[];
  operation?: string;
}
export type AssistantCriteria = components["schemas"]["AssistantCriteria"];
export type AssistantDestination = components["schemas"]["AssistantDestination"];
export type AssistantCandidate = components["schemas"]["AssistantCandidate"];
export type AssistantLabel = components["schemas"]["AssistantLabel"];
export type AssistantLabelKind = components["schemas"]["AssistantLabelKind"];
export type AssistantProposal = components["schemas"]["AssistantProposal"];
export type AssistantProposalStep = components["schemas"]["AssistantProposalStep"];

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
  const response = await request<{ outcome: AssistantOutcome }>("/api/assistant/turns", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  return response.outcome;
}

export type AssistantProposalStatus = AssistantProposal["status"];

/** What Execute led to: approved now, already past proposed (the 409 body), or gone (404). */
export type ExecuteResult =
  | { kind: "approved"; proposal: AssistantProposal }
  | { kind: "notPending"; proposal: AssistantProposal }
  | { kind: "expired" };

/**
 * Approves the kept steps, by index. Other failures, such as 409 step_needs_skipped, reject
 * with RequestFailed; the card keeps its buttons then.
 */
export async function executeAssistantProposal(
  id: number,
  steps: readonly number[],
): Promise<ExecuteResult> {
  const response = await apiFetch(`/api/assistant/proposals/${String(id)}/execute`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ steps }),
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

export type AssistantReviewRun = components["schemas"]["AssistantReviewRun"];
export type AssistantReviewKind = AssistantReviewRun["kind"];
export type AssistantReviewCandidate = components["schemas"]["AssistantReviewCandidate"];
export type AssistantReviewMember = components["schemas"]["AssistantReviewMember"];
export type AssistantReviewRunPage = components["schemas"]["AssistantReviewRunPage"];
export type AssistantReviewAdoption = components["schemas"]["AssistantReviewAdoption"];
export type AssistantReviewRejection = components["schemas"]["AssistantReviewRejection"];

/** The run and one page of its candidates, most probable first; 404 when the run is gone. */
export function getAssistantReviewRun(
  id: number,
  page: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<AssistantReviewRunPage> {
  const query = new URLSearchParams();
  if (page.cursor !== undefined) query.set("cursor", page.cursor);
  if (page.limit !== undefined) query.set("limit", String(page.limit));
  const search = query.toString();
  return request<AssistantReviewRunPage>(
    `/api/assistant/review-runs/${String(id)}${search === "" ? "" : `?${search}`}`,
    { signal },
  );
}

function postCandidates<T>(
  id: number,
  action: "adopt" | "reject",
  ids: readonly number[],
): Promise<T> {
  return request<T>(`/api/assistant/review-runs/${String(id)}/candidates/${action}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
}

/** One approved proposal per adopted candidate, in the order of ids, without the skipped ones. */
export function adoptAssistantReviewCandidates(
  id: number,
  ids: readonly number[],
): Promise<AssistantReviewAdoption> {
  return postCandidates(id, "adopt", ids);
}

/** Rejected candidates are remembered, so a later run of the kind does not list them. */
export function rejectAssistantReviewCandidates(
  id: number,
  ids: readonly number[],
): Promise<AssistantReviewRejection> {
  return postCandidates(id, "reject", ids);
}
