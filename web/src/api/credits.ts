import { apiFetch, request, RequestFailed, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// People and organizations share the same route shape; select their prefix by kind.
//
export type Person = components["schemas"]["Person"];
export type Organization = components["schemas"]["Organization"];
export type CreditInput = components["schemas"]["CreditInput"];
export type CreditRef = components["schemas"]["CreditRef"];
export type CreditPatch = components["schemas"]["CreditPatch"];
export type CreditSort = components["schemas"]["CreditSort"];
export type Alias = components["schemas"]["Alias"];
export type ExternalId = components["schemas"]["ExternalId"];

export type CreditKind = "person" | "organization";

export type Credit = Person | Organization;

export interface CreditList {
  items: Credit[];
  total: number;
  totalAll: number;
  nextCursor?: string;
}

export const maxCreditQueryLength = 100;

function basePath(kind: CreditKind): string {
  return kind === "person" ? "/api/people" : "/api/organizations";
}

function itemPath(kind: CreditKind, id: number): string {
  return `${basePath(kind)}/${String(id)}`;
}

export interface CreditListQuery {
  q?: string;
  sort?: CreditSort;
  /** 1..200 results; omit to retrieve every match. */
  limit?: number;
  cursor?: string;
}

/** Truncate queries to the contract's 100-code-point limit; longer names still match their prefixes. */
export function listCredits(
  kind: CreditKind,
  query: CreditListQuery = {},
  signal?: AbortSignal,
): Promise<CreditList> {
  const params = new URLSearchParams();
  const q = Array.from(query.q ?? "")
    .slice(0, maxCreditQueryLength)
    .join("");
  if (q !== "") params.set("q", q);
  if (query.sort !== undefined) params.set("sort", query.sort);
  if (query.limit !== undefined) params.set("limit", String(query.limit));
  if (query.cursor !== undefined && query.cursor !== "")
    params.set("cursor", query.cursor);
  const search = params.toString();
  return request<CreditList>(`${basePath(kind)}${search === "" ? "" : `?${search}`}`, {
    signal,
  });
}

export function createCredit(
  kind: CreditKind,
  input: CreditInput,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(basePath(kind), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal,
  });
}

export function getCredit(
  kind: CreditKind,
  id: number,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(itemPath(kind, id), { signal });
}

/** Replace only supplied properties, but replace arrays in full; null clears values or removes the parent. */
export function updateCredit(
  kind: CreditKind,
  id: number,
  patch: CreditPatch,
  signal?: AbortSignal,
): Promise<Credit> {
  return request<Credit>(itemPath(kind, id), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
    signal,
  });
}

export interface CreditMergeResult {
  credit: Credit;
  notFoundIds: number[];
}

/** Move names/aliases, external IDs, and video links from sources into the target. */
export async function mergeCredits(
  kind: CreditKind,
  id: number,
  sourceIds: readonly number[],
  signal?: AbortSignal,
): Promise<CreditMergeResult> {
  const body = await request<{
    person?: Credit;
    organization?: Credit;
    notFoundIds: number[];
  }>(`${itemPath(kind, id)}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourceIds }),
    signal,
  });
  const credit = kind === "person" ? body.person : body.organization;
  if (credit === undefined) throw new RequestFailed(500, "", "");
  return { credit, notFoundIds: body.notFoundIds };
}

export async function deleteCredit(
  kind: CreditKind,
  id: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(itemPath(kind, id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/** The face URL; the revision only busts caches, so a changed face is fetched again. */
export function personImagePath(id: number, revision: number): string {
  return `/api/people/${String(id)}/image?v=${String(revision)}`;
}

/** Let the browser set the multipart boundary; the server fits and stores the image. */
export function uploadPersonImage(
  id: number,
  image: Blob,
  signal?: AbortSignal,
): Promise<Person> {
  const form = new FormData();
  form.append("image", image, image instanceof File ? image.name : "image");
  return request<Person>(`${itemPath("person", id)}/image`, {
    method: "PUT",
    body: form,
    signal,
  });
}

export function removePersonImage(id: number, signal?: AbortSignal): Promise<Person> {
  return request<Person>(`${itemPath("person", id)}/image`, { method: "DELETE", signal });
}

/** Match missing target entities only; field-qualified missing parents are input errors. */
export function isCreditGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    (error.reason === "person_not_found" || error.reason === "organization_not_found") &&
    error.field === undefined
  );
}
