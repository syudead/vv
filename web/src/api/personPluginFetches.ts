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

/** What a fetch runs with: a search candidate's ID, or a link the person has or the owner typed. */
export type PersonFetchKey = { source: string; id: string } | { link: string };

/** Without a key the server picks the person's stored ID or one handled link, or answers with a 409. */
export function fetchPersonWithPlugin(
  personId: number,
  pluginId: number,
  key?: PersonFetchKey,
  signal?: AbortSignal,
): Promise<PersonPluginFetch> {
  const body: components["schemas"]["PersonPluginFetchRequest"] = { pluginId, ...key };
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
