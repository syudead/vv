import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

export type ActivityPage = components["schemas"]["ActivityPage"];
export type ActivityEntry = components["schemas"]["ActivityEntry"];
export type ActivityKind = components["schemas"]["ActivityKind"];
export type ActivityFeature = components["schemas"]["ActivityFeature"];
export type ActivityStatus = components["schemas"]["ActivityStatus"];
export type ActivityReason = components["schemas"]["ActivityReason"];
export type ActivityOutcome = components["schemas"]["ActivityOutcome"];
export type ActivityDecider = components["schemas"]["ActivityDecider"];
export type ActivityStage = components["schemas"]["ActivityStage"];
export type ActivityCounts = components["schemas"]["ActivityCounts"];
export type ActivityFeatureCounts = components["schemas"]["ActivityFeatureCounts"];
export type ActivityUndoRequest = components["schemas"]["ActivityUndoRequest"];
export type ActivityTag = components["schemas"]["ActivityTag"];
export type ActivityDetail = components["schemas"]["ActivityDetail"];
export type ActivityStageState = components["schemas"]["ActivityStageState"];
export type ActivityStageRow = components["schemas"]["ActivityStageRow"];
export type ActivityPipelineRef = components["schemas"]["ActivityPipelineRef"];
export type ActivitySubjectItem = components["schemas"]["ActivitySubjectItem"];

export const ACTIVITY_PAGE_SIZE = 50;

export interface ActivityConditions {
  status?: ActivityStatus;
  features?: readonly ActivityFeature[];
  decider?: ActivityDecider;
  /** Inclusive creation-time lower bound, in ISO 8601. */
  since?: string;
  q?: string;
}

export interface ListActivityParams extends ActivityConditions {
  /** Opaque nextCursor from the previous response. */
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

export function activityQuery({
  status,
  features,
  decider,
  since,
  q,
  cursor,
  limit = ACTIVITY_PAGE_SIZE,
}: Omit<ListActivityParams, "signal">): URLSearchParams {
  const params = new URLSearchParams();
  if (status !== undefined) params.set("status", status);
  for (const feature of features ?? []) params.append("feature", feature);
  if (decider !== undefined) params.set("decider", decider);
  if (since !== undefined) params.set("since", since);
  if (q !== undefined && q !== "") params.set("q", q);
  if (cursor !== undefined) params.set("cursor", cursor);
  params.set("limit", String(limit));
  return params;
}

/** Counts ignore status filtering; feature counts ignore feature filtering. */
export function listActivity({
  signal,
  ...params
}: ListActivityParams = {}): Promise<ActivityPage> {
  return request<ActivityPage>(`/api/activity?${activityQuery(params).toString()}`, {
    signal,
  });
}

/** Read one entry of any kind with its full detail; a missing entry rejects with status 404. */
export function getActivity(id: number, signal?: AbortSignal): Promise<ActivityDetail> {
  return request<ActivityDetail>(`/api/activity/${String(id)}`, { signal });
}

function activityPath(id: number, action: "retry" | "undo"): string {
  return `/api/activity/${String(id)}/${action}`;
}

/** Retry creates a new activity record; activity events trigger refresh. */
export async function retryActivity(id: number): Promise<void> {
  const response = await apiFetch(activityPath(id, "retry"), { method: "POST" });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Undo one auto-tag assignment using tagId; invalidate list snapshots after success. */
export async function undoActivity(id: number, tagId?: number): Promise<ActivityEntry> {
  const body: ActivityUndoRequest = tagId === undefined ? {} : { tagId };
  const entry = await request<ActivityEntry>(activityPath(id, "undo"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  clearListSnapshot();
  return entry;
}
