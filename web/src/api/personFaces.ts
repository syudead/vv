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

function pairPath(id: number, otherId: number, action: "confirm" | "dismiss"): string {
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

/** Merge the provisional one of the pair into the other; returns the merged person. */
export function confirmFaceCandidate(id: number, otherId: number): Promise<Person> {
  return request<Person>(pairPath(id, otherId, "confirm"), { method: "POST" });
}

export async function dismissFaceCandidate(id: number, otherId: number): Promise<void> {
  const response = await apiFetch(pairPath(id, otherId, "dismiss"), { method: "POST" });
  if (!response.ok) throw await toRequestFailed(response);
}

/** True when the pair is no longer open, so its row has nothing left to decide. */
export function isCandidateGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "face_candidate_not_found"
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
