import { apiFetch, RequestFailed, request, toRequestFailed } from "./client";
import { clearListSnapshot } from "./listSnapshot";
import {
  afterSmartFolderMembersChanged,
  type VideoTargetOptions,
  smartFolderField,
} from "./smartFolderChanges";
import { nextVideoTagsSequence, recordAppliedVideoTags } from "./videoTagsEvents";
import type { components } from "./gen/openapi";

export type Tag = components["schemas"]["Tag"];
export type TagRef = components["schemas"]["TagRef"];
export type VideoTagsResponse = components["schemas"]["VideoTagsResponse"];
export type VideoTagsSummary = components["schemas"]["VideoTagsSummary"];
export type RejectedTagNameList = components["schemas"]["RejectedTagNameList"];
export type TagBatchAction = components["schemas"]["TagBatchRequest"]["action"];
/** Only setKind sends kindId. */
export type TagBatchCommand =
  Exclude<TagBatchAction, "setKind"> | { action: "setKind"; kindId: number };
export type TagBatchResponse = components["schemas"]["TagBatchResponse"];
export type TagImpactAction = components["schemas"]["TagImpactRequest"]["action"];
export type TagImpactResponse = components["schemas"]["TagImpactResponse"];
export type TagMergeResponse = components["schemas"]["TagMergeResponse"];
export type TagList = components["schemas"]["TagList"];
export type TagSort = components["schemas"]["TagSort"];

/** Batch updates are atomic: disable oversized selections instead of silently splitting requests. */
export const maxVideoTagsSelection = 20000;

/** Disable Select all or batch actions when their respective counts exceed the API limit. */
export const maxTagBatch = 20000;

/** Shared reads accept no per-caller AbortSignal so one caller cannot cancel other waiters. */
function listTags(): Promise<Tag[]> {
  return request<TagList>("/api/tags").then((page) => page.items);
}

export const tagPageLimit = 100;

export interface TagPageQuery {
  /** Match normalized names and synonyms; empty means no filter. */
  q?: string;
  tentative?: boolean;
  unused?: boolean;
  /** Positive ID string, or none for tags without a kind. */
  kind?: string;
  sort?: TagSort;
  /** Reuse nextCursor only with identical conditions. */
  cursor?: string;
  limit?: number;
}

/** Server-side filtered pagination does not touch the shared cache and accepts caller-owned cancellation. */
export function listTagPage(query: TagPageQuery, signal?: AbortSignal): Promise<TagList> {
  const params = new URLSearchParams();
  if (query.q !== undefined && query.q !== "") params.set("q", query.q);
  if (query.tentative === true) params.set("tentative", "true");
  if (query.unused === true) params.set("unused", "true");
  if (query.kind !== undefined) params.set("kind", query.kind);
  if (query.sort !== undefined) params.set("sort", query.sort);
  if (query.cursor !== undefined && query.cursor !== "")
    params.set("cursor", query.cursor);
  params.set("limit", String(query.limit ?? tagPageLimit));
  return request<TagList>(`/api/tags?${params.toString()}`, { signal });
}

/** Share the latest list so candidates and filters observe mutations consistently across mounted screens. */
type TagsListener = (tags: Tag[]) => void;

let held: Tag[] | undefined;
let generation = 0;
/** Superseded requests await the winning fetch rather than return stale data, even before held exists. */
let latestFetch: Promise<Tag[]> | undefined;
/** Coalesce concurrent getTags calls only while the cache is empty. */
let pendingGet: Promise<Tag[]> | undefined;
/** Requests at or before this invalidation generation must not repopulate held. */
let droppedGeneration = 0;
let latestPending = false;
let heldAt = 0;
const listeners = new Set<TagsListener>();

/** Reuse recent lists on mount to avoid redundant reads from components opening together. */
const revalidateWindowMs = 2000;

function notify(tags: Tag[]): void {
  for (const listener of listeners) listener(tags);
}

/** Always start independently; superseded requests follow the winning fetch until one actually updates held. Never return an obsolete response directly. */
function startFetch(): Promise<Tag[]> {
  generation += 1;
  const myGeneration = generation;
  latestPending = true;
  const fetchPromise: Promise<Tag[]> = listTags().then((tags): Tag[] | Promise<Tag[]> => {
    if (myGeneration === generation) latestPending = false;
    if (myGeneration !== generation) {
      // A newer startFetch installs latestFetch immediately after advancing generation.
      return latestFetch!;
    }
    // Pre-invalidation reads must return a fresh result to waiting callers, not their obsolete response.
    if (myGeneration <= droppedGeneration) return startFetch();
    held = tags;
    heldAt = Date.now();
    notify(tags);
    return tags;
  });
  fetchPromise.catch(() => {
    if (myGeneration === generation) latestPending = false;
  });
  latestFetch = fetchPromise;
  return fetchPromise;
}

/** Reuse held or coalesce initial reads; explicit refreshTags calls remain independent. */
export function getTags(): Promise<Tag[]> {
  if (held !== undefined) return Promise.resolve(held);
  if (pendingGet === undefined) {
    const fetching: Promise<Tag[]> = startFetch().finally(() => {
      // Do not clear coalescing state for a request started after invalidation.
      if (pendingGet === fetching) pendingGet = undefined;
    });
    pendingGet = fetching;
  }
  return pendingGet;
}

export function currentTags(): Tag[] | undefined {
  return held;
}

/** Always start a new request, even when another read is pending. */
export function refreshTags(): Promise<Tag[]> {
  return startFetch();
}

/** Reuse pending/recent reads on mount; after mutations use refreshTags so the request starts after the change. */
export function revalidateTags(): Promise<Tag[]> {
  if (latestPending && latestFetch !== undefined) return latestFetch;
  if (held !== undefined && Date.now() - heldAt < revalidateWindowMs) {
    return Promise.resolve(held);
  }
  return startFetch();
}

export function subscribeTags(listener: TagsListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Reset shared state between tests only; production retains it for the tab's lifetime. */
export function __resetTagsForTest(): void {
  held = undefined;
  generation = 0;
  latestFetch = undefined;
  latestPending = false;
  heldAt = 0;
  pendingGet = undefined;
  droppedGeneration = 0;
  listeners.clear();
}

/** Refresh stale-tag errors asynchronously without unhandled rejections; always rethrow the original error. */
function refreshOnStaleTagError(error: unknown): never {
  if (
    error instanceof RequestFailed &&
    (error.code === "tag_not_found" ||
      error.code === "tag_merge_required" ||
      error.code === "tag_not_tentative")
  ) {
    refreshTags().catch(() => undefined);
  }
  throw error;
}

/** New tags can match existing ancestor folders, so invalidate video snapshots as well as the tag cache. */
function afterTagCreated(): void {
  afterTagChanged();
}

/** Name-based application may create tags; refresh counts/candidates and invalidate snapshots for new folder-derived assignments. */
export function afterTagsAppliedByName(): void {
  afterTagCreated();
}

/** Invalidate snapshots and refresh tags only with subscribers; otherwise defer reads until the next getTags. */
function afterTagChanged(): void {
  clearListSnapshot();
  if (listeners.size > 0) {
    refreshTags().catch(() => undefined);
    return;
  }
  dropHeldTags();
}

/** Prevent pre-invalidation requests from repopulating held; their callers receive fresh results instead. */
function dropHeldTags(): void {
  held = undefined;
  pendingGet = undefined;
  droppedGeneration = generation;
}

/** Publish a bulk edit's tag changes to snapshots and lists; a tag created by name also invalidates snapshots. */
export function afterBulkTagEdit(
  videoIds: readonly number[],
  {
    added,
    removed,
    created,
  }: { added: readonly TagRef[]; removed: readonly TagRef[]; created: boolean },
  sequence: number,
): void {
  for (const tag of added) recordAppliedVideoTags(videoIds, tag, "add", sequence);
  for (const tag of removed) recordAppliedVideoTags(videoIds, tag, "remove", sequence);
  if (created) afterTagCreated();
  else if (added.length + removed.length > 0) refreshTagCounts();
}

/** Rethrow a request error after refreshing the tag cache when it names a stale tag. */
export function rethrowStaleTagError(error: unknown): never {
  return refreshOnStaleTagError(error);
}

/** Refresh counts without invalidating video snapshots; ignore background refresh failures. */
function refreshTagCounts(): void {
  refreshTags().catch(() => undefined);
}

export async function createTag(name: string, signal?: AbortSignal): Promise<Tag> {
  const created = await request<Tag>("/api/tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagCreated();
  return created;
}

export async function renameTag(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<Tag> {
  const updated = await request<Tag>(`/api/tags/${String(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
  return updated;
}

export async function deleteTag(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/tags/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}

/** Missing sources are reported but do not fail merging; all-missing sources leave the target unchanged. */
export async function mergeTag(
  id: number,
  sourceIds: readonly number[],
  signal?: AbortSignal,
): Promise<TagMergeResponse> {
  const merged = await request<TagMergeResponse>(`/api/tags/${String(id)}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sourceIds }),
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
  return merged;
}

/** Supply mergeTagId only after confirming the conflicting tag; unexpected conflicts return tag_merge_required. */
export async function addTagSynonym(
  id: number,
  name: string,
  mergeTagId?: number,
  signal?: AbortSignal,
): Promise<Tag> {
  const updated = await request<Tag>(`/api/tags/${String(id)}/synonyms`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mergeTagId === undefined ? { name } : { name, mergeTagId }),
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
  return updated;
}

export async function removeTagSynonym(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<void> {
  const query = new URLSearchParams({ name });
  const response = await apiFetch(
    `/api/tags/${String(id)}/synonyms?${query.toString()}`,
    {
      method: "DELETE",
      signal,
    },
  );
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}

/** Confirming an already confirmed tag returns its current state. */
export async function confirmTag(id: number, signal?: AbortSignal): Promise<Tag> {
  const confirmed = await request<Tag>(`/api/tags/${String(id)}/confirm`, {
    method: "POST",
    signal,
  }).catch(refreshOnStaleTagError);
  afterTagChanged();
  return confirmed;
}

/** Rejecting deletes the tag and records its name; confirmed tags fail. Callers refresh the rejected-name list. */
export async function rejectTag(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/tags/${String(id)}/reject`, {
    method: "POST",
    signal,
  });
  if (!response.ok) {
    refreshOnStaleTagError(await toRequestFailed(response));
  }
  afterTagChanged();
}

/** Report skipped/missing IDs and invalidate caches once after success; callers refresh rejected names after rejection. */
export async function batchTags(
  command: TagBatchCommand,
  ids: readonly number[],
  signal?: AbortSignal,
): Promise<TagBatchResponse> {
  const action = typeof command === "string" ? { action: command } : command;
  const result = await request<TagBatchResponse>("/api/tags/batch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...action, ids: Array.from(ids) }),
    signal,
  });
  afterTagChanged();
  return result;
}

/** Read-only impact calculation; do not invalidate shared state. */
export function tagImpact(
  action: TagImpactAction,
  ids: readonly number[],
  signal?: AbortSignal,
): Promise<TagImpactResponse> {
  return request<TagImpactResponse>("/api/tags/impact", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, ids: Array.from(ids) }),
    signal,
  });
}

/** Paginated rejected names bypass the shared cache. */
export function listRejectedTagNamePage(
  cursor?: string,
  limit?: number,
  signal?: AbortSignal,
): Promise<RejectedTagNameList> {
  const query = new URLSearchParams();
  if (cursor !== undefined) query.set("cursor", cursor);
  if (limit !== undefined) query.set("limit", String(limit));
  const search = query.toString();
  return request<RejectedTagNameList>(
    search === "" ? "/api/tags/rejected-names" : `/api/tags/rejected-names?${search}`,
    { signal },
  );
}

/** Forgetting absent names succeeds; future automatic assignments may recreate the tentative tag. */
export async function forgetRejectedTagName(
  name: string,
  signal?: AbortSignal,
): Promise<void> {
  const query = new URLSearchParams({ name });
  const response = await apiFetch(`/api/tags/rejected-names?${query.toString()}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    throw await toRequestFailed(response);
  }
}

async function updateVideoTags(
  videoIds: readonly number[],
  action: "add" | "remove",
  tag: { id: number } | { name: string },
  { smartFolderIds = [], signal }: VideoTargetOptions,
): Promise<VideoTagsResponse> {
  // Allocate before sending so out-of-order responses can be rejected by sequence.
  const sequence = nextVideoTagsSequence();
  const result = await request<VideoTagsResponse>("/api/video-tags", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      videoIds: Array.from(videoIds),
      ...smartFolderField(smartFolderIds),
      action,
      tag,
    }),
    signal,
  }).catch(refreshOnStaleTagError);
  recordAppliedVideoTags(videoIds, result.tag, action, sequence);
  afterSmartFolderMembersChanged(smartFolderIds);
  return result;
}

/** Refresh shared tag counts after successful assignment. */
export function attachVideoTagByID(
  videoIds: readonly number[],
  tagId: number,
  options: VideoTargetOptions = {},
): Promise<VideoTagsResponse> {
  return updateVideoTags(videoIds, "add", { id: tagId }, options).then((result) => {
    refreshTagCounts();
    return result;
  });
}

/** Resolve aliases or create names; newly created tags also invalidate snapshots because folder matching can affect other videos. */
export function attachVideoTagByName(
  videoIds: readonly number[],
  name: string,
  options: VideoTargetOptions = {},
): Promise<VideoTagsResponse> {
  const known = currentTags();
  return updateVideoTags(videoIds, "add", { name }, options).then((result) => {
    if (known?.some((tag) => tag.id === result.tag.id)) {
      refreshTagCounts();
    } else {
      afterTagCreated();
    }
    return result;
  });
}

/** Remove by ID and refresh shared counts. */
export function detachVideoTag(
  videoIds: readonly number[],
  tagId: number,
  options: VideoTargetOptions = {},
): Promise<VideoTagsResponse> {
  return updateVideoTags(videoIds, "remove", { id: tagId }, options).then((result) => {
    refreshTagCounts();
    return result;
  });
}

export function summarizeVideoTags(
  videoIds: readonly number[],
  { smartFolderIds = [], signal }: VideoTargetOptions = {},
): Promise<VideoTagsSummary> {
  return request<VideoTagsSummary>("/api/video-tags/summary", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      videoIds: Array.from(videoIds),
      ...smartFolderField(smartFolderIds),
    }),
    signal,
  });
}
