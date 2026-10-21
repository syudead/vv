import { apiFetch, request, RequestFailed, toRequestFailed } from "./client";
import type { Person } from "./credits";
import type { components } from "./gen/openapi";

export type FaceCandidate = components["schemas"]["FaceCandidate"];
export type FaceCandidatePerson = components["schemas"]["FaceCandidatePerson"];
export type FaceAppearance = components["schemas"]["FaceAppearance"];
export type FaceAppearanceList = components["schemas"]["FaceAppearanceList"];

function personPath(id: number): string {
  return `/api/people/${String(id)}`;
}

function pairPath(
  id: number,
  otherId: number,
  action: "confirm" | "dismiss" | "reopen",
): string {
  return `${personPath(id)}/face-candidates/${String(otherId)}/${action}`;
}

/** Open pairs the person takes part in, highest similarity first, each as the other person. */
export async function listFaceCandidates(
  id: number,
  signal?: AbortSignal,
): Promise<FaceCandidate[]> {
  const body = await request<components["schemas"]["FaceCandidateList"]>(
    `${personPath(id)}/face-candidates`,
    { signal },
  );
  return body.items;
}

/** Merge the other person into id, or id into a named other when id has no name; returns the merged person. */
export function confirmFaceCandidate(id: number, otherId: number): Promise<Person> {
  return request<Person>(pairPath(id, otherId, "confirm"), { method: "POST" });
}

async function postPair(
  id: number,
  otherId: number,
  action: "dismiss" | "reopen",
): Promise<void> {
  const response = await apiFetch(pairPath(id, otherId, action), { method: "POST" });
  if (!response.ok) throw await toRequestFailed(response);
}

export function dismissFaceCandidate(id: number, otherId: number): Promise<void> {
  return postPair(id, otherId, "dismiss");
}

/** Takes back a dismissal, so the pair is offered again. */
export function reopenFaceCandidate(id: number, otherId: number): Promise<void> {
  return postPair(id, otherId, "reopen");
}

/** True when the pair is no longer open, so its row has nothing left to decide. */
export function isCandidateGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "face_candidate_not_found"
  );
}

/** Split the appearance into a new provisional person; returns that person. */
export function splitFaceAppearance(id: number, appearanceId: number): Promise<Person> {
  return request<Person>(`${personPath(id)}/appearances/${String(appearanceId)}/split`, {
    method: "POST",
  });
}

/** True when the appearance is no longer linked to the person, so its row has nothing left to split. */
export function isAppearanceGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "face_appearance_not_found"
  );
}

export function listPersonAppearances(
  id: number,
  query: { limit: number; cursor?: string },
  signal?: AbortSignal,
): Promise<FaceAppearanceList> {
  const params = new URLSearchParams({ limit: String(query.limit) });
  if (query.cursor !== undefined) params.set("cursor", query.cursor);
  return request<FaceAppearanceList>(
    `${personPath(id)}/appearances?${params.toString()}`,
    {
      signal,
    },
  );
}

export function faceCropPath(appearanceId: number): string {
  return `/api/face-appearances/${String(appearanceId)}/crop`;
}
