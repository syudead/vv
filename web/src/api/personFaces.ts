import { apiFetch, request, RequestFailed, toRequestFailed } from "./client";
import type { Person } from "./credits";
import type { components } from "./gen/openapi";

export type FaceCandidate = components["schemas"]["FaceCandidate"];
export type FaceCandidatePerson = components["schemas"]["FaceCandidatePerson"];
export type FaceAppearance = components["schemas"]["FaceAppearance"];
export type FaceAppearanceList = components["schemas"]["FaceAppearanceList"];
export type SimilarPerson = components["schemas"]["SimilarPerson"];

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

/** Merge otherId into id, or id into otherId when only otherId has a name; returns the merged person. */
export type FaceCandidateConfirmation =
  components["schemas"]["FaceCandidateConfirmation"];

export function confirmFaceCandidate(
  id: number,
  otherId: number,
): Promise<FaceCandidateConfirmation> {
  return request<FaceCandidateConfirmation>(pairPath(id, otherId, "confirm"), {
    method: "POST",
  });
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

/** Split several appearances into one new provisional person; returns that person. */
export function splitFaceAppearances(
  id: number,
  appearanceIds: readonly number[],
): Promise<Person> {
  return request<Person>(`${personPath(id)}/appearances/split`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ appearanceIds }),
  });
}

export type FaceAppearanceMove = components["schemas"]["FaceAppearanceMove"];
export type FaceMoveCredit = components["schemas"]["FaceMoveCredit"];

/**
 * Move appearances of the person to another person; returns that person with the credits only the reverse move puts
 * back, which an undo passes as restoreCredits.
 */
export function moveFaceAppearances(
  id: number,
  appearanceIds: readonly number[],
  targetId: number,
  restoreCredits: readonly FaceMoveCredit[] = [],
): Promise<FaceAppearanceMove> {
  return request<FaceAppearanceMove>(`${personPath(id)}/appearances/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ appearanceIds, targetId, restoreCredits }),
  });
}

/** Other people whose faces look like the given appearances of the person, or all of them; most similar first. */
export async function listSimilarPeople(
  id: number,
  appearanceIds: readonly number[],
  signal?: AbortSignal,
): Promise<SimilarPerson[]> {
  const params = new URLSearchParams();
  for (const appearanceId of appearanceIds)
    params.append("appearanceId", String(appearanceId));
  const query = params.size === 0 ? "" : `?${params.toString()}`;
  const body = await request<components["schemas"]["SimilarPersonList"]>(
    `${personPath(id)}/similar-people${query}`,
    { signal },
  );
  return body.items;
}

/** True when the appearance is no longer linked to the person, so its row has nothing left to split. */
export function isAppearanceGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "face_appearance_not_found"
  );
}

export type FaceAppearanceSort = "newest" | "leastSimilar";

export function listPersonAppearances(
  id: number,
  query: { limit?: number; cursor?: string; sort?: FaceAppearanceSort },
  signal?: AbortSignal,
): Promise<FaceAppearanceList> {
  const params = new URLSearchParams();
  if (query.limit !== undefined) params.set("limit", String(query.limit));
  if (query.sort !== undefined) params.set("sort", query.sort);
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
