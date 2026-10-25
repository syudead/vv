import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { smartFolderField } from "./smartFolderChanges";
import { afterTagsAppliedByName } from "./tags";

// Owner-only plugin changes emit no events; refresh the list after mutations.

export type WorkInfoPlugin = components["schemas"]["WorkInfoPlugin"];
export type WorkInfoPluginSetting = components["schemas"]["WorkInfoPluginSetting"];
export type WorkInfoPluginState = components["schemas"]["WorkInfoPluginState"];
export type WorkInfoPluginKind = components["schemas"]["WorkInfoPluginKind"];
export type WorkInfoPluginProblem = components["schemas"]["WorkInfoPluginProblem"];
export type WorkInfoPluginList = components["schemas"]["WorkInfoPluginList"];
export type WorkInfoPluginRun = components["schemas"]["WorkInfoPluginRun"];
export type WorkInfoPluginRunFailure = components["schemas"]["WorkInfoPluginRunFailure"];
export type WorkInfoCandidate = components["schemas"]["WorkInfoCandidate"];
export type WorkInfoFetch = components["schemas"]["WorkInfoFetch"];
export type WorkInfoField = components["schemas"]["WorkInfoField"];
export type WorkInfoFetchApplied = components["schemas"]["WorkInfoFetchApplied"];
type ApplyWorkInfoFetchRequest = components["schemas"]["ApplyWorkInfoFetchRequest"];
export type SkippedTag = components["schemas"]["SkippedTag"];
export type WorkInfoFetchedDate = components["schemas"]["WorkInfoFetchedDate"];
export type WorkInfoFetchedPerson = components["schemas"]["WorkInfoFetchedPerson"];
export type WorkInfoFetchedOrganization =
  components["schemas"]["WorkInfoFetchedOrganization"];
export type WorkInfoMatch = components["schemas"]["WorkInfoMatch"];
export type WorkInfoMatchState = components["schemas"]["WorkInfoMatchState"];
export type SkippedWorkInfoValue = components["schemas"]["SkippedWorkInfoValue"];
export type SkippedWorkInfoReason = components["schemas"]["SkippedWorkInfoReason"];
export type WorkInfoFillsQueued = components["schemas"]["WorkInfoFillsQueued"];
export type WorkInfoFillTarget = components["schemas"]["WorkInfoFillTarget"];
export type WorkInfoFillItem = components["schemas"]["WorkInfoFillItem"];
export type WorkInfoSearchResult = components["schemas"]["WorkInfoSearchResult"];

const jsonHeaders = { "Content-Type": "application/json" };

function pluginPath(id: number): string {
  return `/api/work-info-plugins/${String(id)}`;
}

export async function listWorkInfoPlugins(
  signal?: AbortSignal,
): Promise<WorkInfoPlugin[]> {
  const list = await request<WorkInfoPluginList>("/api/work-info-plugins", { signal });
  return list.plugins;
}

/** Let the browser set multipart boundaries; distinguish replacement by HTTP 200 versus creation by 201, not cached list state. */
export async function uploadWorkInfoPlugin(
  module: File,
  signal?: AbortSignal,
): Promise<{ plugin: WorkInfoPlugin; replaced: boolean }> {
  const form = new FormData();
  form.append("module", module, module.name);
  const response = await apiFetch("/api/work-info-plugins", {
    method: "POST",
    body: form,
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  const plugin = (await response.json()) as WorkInfoPlugin;
  return { plugin, replaced: response.status === 200 };
}

/** Approve the observed digest only; replaced or unusable modules fail with 409. */
export function approveWorkInfoPlugin(
  id: number,
  digest: string,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  return request<WorkInfoPlugin>(`${pluginPath(id)}/approval`, {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ digest }),
    signal,
  });
}

export function revokeWorkInfoPluginApproval(
  id: number,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  return request<WorkInfoPlugin>(`${pluginPath(id)}/approval`, {
    method: "DELETE",
    signal,
  });
}

/** Null clears a setting; omitted keys and approval remain unchanged. Secret responses expose only whether values are set. */
export function setWorkInfoPluginSettings(
  id: number,
  values: Readonly<Record<string, string | null>>,
  signal?: AbortSignal,
): Promise<WorkInfoPlugin> {
  return request<WorkInfoPlugin>(`${pluginPath(id)}/settings`, {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ values }),
    signal,
  });
}

/** pluginIds names every plugin with a linkRank, first tried first; a stale set fails with 409 link_order_changed. */
export async function setWorkInfoPluginLinkOrder(
  pluginIds: readonly number[],
  signal?: AbortSignal,
): Promise<WorkInfoPlugin[]> {
  const list = await request<WorkInfoPluginList>("/api/work-info-plugins/link-order", {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ pluginIds }),
    signal,
  });
  return list.plugins;
}

export async function deleteWorkInfoPlugin(
  id: number,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(pluginPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Return current-module stderr with secrets redacted, or null if no run remains. */
export async function getWorkInfoPluginLastRun(
  id: number,
  signal?: AbortSignal,
): Promise<WorkInfoPluginRun | null> {
  const body = await request<components["schemas"]["WorkInfoPluginLastRun"]>(
    `${pluginPath(id)}/last-run`,
    { signal },
  );
  return body.run ?? null;
}

export async function searchWorkInfo(
  id: number,
  query: string,
  signal?: AbortSignal,
): Promise<WorkInfoCandidate[]> {
  return (await searchWorkInfoMatch(id, query, signal)).candidates;
}

/** Search like searchWorkInfo, with the index of the candidate a fill preselects in `match`. */
export function searchWorkInfoMatch(
  id: number,
  query: string,
  signal?: AbortSignal,
): Promise<WorkInfoSearchResult> {
  return request<WorkInfoSearchResult>(`${pluginPath(id)}/search`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ query }),
    signal,
  });
}

/** One video per user-data key among the videos and smart folders, with its title and search words. */
export async function listWorkInfoFillTargets(
  videoIds: readonly number[],
  smartFolderIds: readonly number[],
  signal?: AbortSignal,
): Promise<WorkInfoFillTarget[]> {
  const list = await request<components["schemas"]["WorkInfoFillTargetList"]>(
    "/api/work-info-fill-targets",
    {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ videoIds, ...smartFolderField(smartFolderIds) }),
      signal,
    },
  );
  return list.targets;
}

/** Queue fills of each video from its chosen work; the server answers before any plugin runs. */
export function startWorkInfoFills(
  pluginId: number,
  items: readonly WorkInfoFillItem[],
  signal?: AbortSignal,
): Promise<WorkInfoFillsQueued> {
  return request<WorkInfoFillsQueued>(`${pluginPath(pluginId)}/fills`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ items }),
    signal,
  });
}

/** Treat id as opaque even when URL-shaped; plugins build URLs. Fetch results last ten minutes without modifying the video. */
export function fetchWorkInfo(
  videoId: number,
  pluginId: number,
  id: string,
  signal?: AbortSignal,
): Promise<WorkInfoFetch> {
  return request<WorkInfoFetch>(`/api/videos/${String(videoId)}/work-info-fetches`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ pluginId, id }),
    signal,
  });
}

/**
 * Fetches by a link instead of an ID, for a plugin that declares domains; a link none of them matches answers 400
 * work_info_link_unsupported before the plugin runs.
 */
export function fetchWorkInfoByLink(
  videoId: number,
  pluginId: number,
  link: string,
  signal?: AbortSignal,
): Promise<WorkInfoFetch> {
  return request<WorkInfoFetch>(`/api/videos/${String(videoId)}/work-info-fetches`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ pluginId, link }),
    signal,
  });
}

/** Fetch covers through the server; browsers never contact the external site directly. */
export function workInfoFetchCoverUrl(
  videoId: number,
  fetchId: string,
  index: number,
): string {
  return `${fetchPath(videoId, fetchId)}/covers/${String(index)}`;
}

/** Read a cover's bytes so the page can frame them before they become the thumbnail. */
export async function getWorkInfoFetchCover(
  videoId: number,
  fetchId: string,
  index: number,
  signal?: AbortSignal,
): Promise<Blob> {
  const response = await apiFetch(workInfoFetchCoverUrl(videoId, fetchId, index), {
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  return response.blob();
}

/** The face kept with the fetch for the person at index in the result's people. */
export function workInfoFetchPersonImageUrl(
  videoId: number,
  fetchId: string,
  index: number,
): string {
  return `${fetchPath(videoId, fetchId)}/people/${String(index)}/image`;
}

function fetchPath(videoId: number, fetchId: string): string {
  return `/api/videos/${String(videoId)}/work-info-fetches/${encodeURIComponent(fetchId)}`;
}

/** What an apply takes; people and organizations are positions in the result. */
export type WorkInfoApplyChoice = Omit<ApplyWorkInfoFetchRequest, "coverByPage">;

/** Apply a selection once; expired results or changed plugins fail as stale. Return skipped entries and refreshed video metadata. coverByPage leaves a selected cover for the page to upload. */
export async function applyWorkInfoFetch(
  videoId: number,
  fetchId: string,
  choice: WorkInfoApplyChoice,
  signal?: AbortSignal,
  coverByPage = false,
): Promise<WorkInfoFetchApplied> {
  const body: ApplyWorkInfoFetchRequest = coverByPage
    ? { ...choice, coverByPage: true }
    : choice;
  const applied = await request<WorkInfoFetchApplied>(
    `/api/videos/${String(videoId)}/work-info-fetches/${encodeURIComponent(fetchId)}/apply`,
    { method: "POST", headers: jsonHeaders, body: JSON.stringify(body), signal },
  );
  // Invalidate detached lists and refresh tags so new names appear in candidates after applying metadata.
  if (choice.fields.includes("tags")) afterTagsAppliedByName();
  else clearListSnapshot();
  return applied;
}
