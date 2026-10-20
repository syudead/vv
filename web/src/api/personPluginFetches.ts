import { request } from "./client";
import type { components } from "./gen/openapi";

// Owner-only person page fetches from person plugins; nothing changes until the apply.

export type PersonPluginCandidate = components["schemas"]["PersonPluginCandidate"];
export type PersonPluginFetch = components["schemas"]["PersonPluginFetch"];
export type PersonPluginFetchedPerson =
  components["schemas"]["PersonPluginFetchedPerson"];
export type PersonPluginItem = components["schemas"]["PersonPluginItem"];
export type SkippedPersonValue = components["schemas"]["SkippedPersonValue"];
export type PersonPluginFetchApplied = components["schemas"]["PersonPluginFetchApplied"];

const jsonHeaders = { "Content-Type": "application/json" };

/** The apply's choice; aliases, external IDs and links are chosen one by one. */
export type PersonPluginApplyChoice = Omit<
  components["schemas"]["ApplyPersonPluginFetchRequest"],
  "imageIndex"
>;

export async function searchPersonWithPlugin(
  pluginId: number,
  query: string,
  signal?: AbortSignal,
): Promise<PersonPluginCandidate[]> {
  const result = await request<components["schemas"]["PersonPluginSearchResult"]>(
    `/api/work-info-plugins/${String(pluginId)}/person-search`,
    { method: "POST", headers: jsonHeaders, body: JSON.stringify({ query }), signal },
  );
  return result.candidates;
}

/** Without a candidate the server picks the person's stored ID, or answers person_external_id_missing. */
export function fetchPersonWithPlugin(
  personId: number,
  pluginId: number,
  candidate?: { source: string; id: string },
  signal?: AbortSignal,
): Promise<PersonPluginFetch> {
  const body: components["schemas"]["PersonPluginFetchRequest"] = { pluginId };
  if (candidate !== undefined) {
    body.source = candidate.source;
    body.id = candidate.id;
  }
  return request<PersonPluginFetch>(`/api/people/${String(personId)}/plugin-fetches`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  });
}

function fetchPath(personId: number, fetchId: string): string {
  return `/api/people/${String(personId)}/plugin-fetches/${encodeURIComponent(fetchId)}`;
}

/** One of the images kept with the result; the browser never contacts the external site. */
export function personPluginFetchImageUrl(
  personId: number,
  fetchId: string,
  index: number,
): string {
  return `${fetchPath(personId, fetchId)}/images/${String(index)}`;
}

export function applyPersonPluginFetch(
  personId: number,
  fetchId: string,
  choice: PersonPluginApplyChoice,
  imageIndex: number | null,
  signal?: AbortSignal,
): Promise<PersonPluginFetchApplied> {
  const body: components["schemas"]["ApplyPersonPluginFetchRequest"] =
    imageIndex === null ? choice : { ...choice, imageIndex };
  return request<PersonPluginFetchApplied>(`${fetchPath(personId, fetchId)}/apply`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  });
}
