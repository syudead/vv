import { apiFetch, RequestFailed, request, toRequestFailed } from "./client";
import { clearListSnapshot } from "./listSnapshot";
import { compareNatural } from "./tagOrder";
import { refreshTags } from "./tags";
import type { components } from "./gen/openapi";

export type TagKind = components["schemas"]["TagKind"];
export type TagKindRef = components["schemas"]["TagKindRef"];
export type TagKindList = components["schemas"]["TagKindList"];

/** Natural case-insensitive name order, then ID for ties; the server returns ID order. */
export function sortTagKinds(kinds: readonly TagKind[]): TagKind[] {
  return [...kinds].sort((a, b) => compareNatural(a.name, b.name) || a.id - b.id);
}

/** Share only the latest tag-kind list across management, menus, and filters; invalidate on mutations and missing-kind errors. */
type TagKindsListener = (kinds: TagKind[]) => void;

let held: TagKind[] | undefined;
let generation = 0;
let latestFetch: Promise<TagKind[]> | undefined;
const listeners = new Set<TagKindsListener>();

/** Shared requests must not accept per-caller abort signals. */
export function listTagKinds(): Promise<TagKind[]> {
  return request<TagKindList>("/api/tag-kinds").then((list) => list.items);
}

/** Superseded responses must neither populate the cache nor reach callers; return the current generation's result instead. */
function startFetch(): Promise<TagKind[]> {
  generation += 1;
  const myGeneration = generation;
  const fetching: Promise<TagKind[]> = listTagKinds().then(
    (kinds): TagKind[] | Promise<TagKind[]> => {
      if (myGeneration !== generation) return latestFetch ?? startFetch();
      held = kinds;
      for (const listener of listeners) listener(kinds);
      return kinds;
    },
  );
  // Clear failed requests so the next reader can retry.
  fetching.catch(() => {
    if (latestFetch === fetching) latestFetch = undefined;
  });
  latestFetch = fetching;
  return fetching;
}

export function getTagKinds(): Promise<TagKind[]> {
  if (held !== undefined) return Promise.resolve(held);
  return latestFetch ?? startFetch();
}

export function currentTagKinds(): TagKind[] | undefined {
  return held;
}

export function refreshTagKinds(): Promise<TagKind[]> {
  return startFetch();
}

export function subscribeTagKinds(listener: TagKindsListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Refresh with subscribers, otherwise invalidate; stale in-flight results cannot repopulate the cache. Do not await refresh failures. */
function afterTagKindsChanged(): void {
  held = undefined;
  latestFetch = undefined;
  generation += 1;
  if (listeners.size > 0) refreshTagKinds().catch(() => undefined);
}

/** Cached tags embed kind names; refresh them and invalidate video-list snapshots after renaming or deletion. */
function afterTagKindRenamedOrDeleted(): void {
  afterTagKindsChanged();
  clearListSnapshot();
  refreshTags().catch(() => undefined);
}

export function tagKindCountsChanged(): void {
  afterTagKindsChanged();
}

export function tagKindGone(): void {
  afterTagKindRenamedOrDeleted();
}

/** Missing kinds also invalidate tag and video-list caches; always rethrow the original error. */
function refreshOnStaleTagKindError(error: unknown): never {
  if (error instanceof RequestFailed && error.reason === "tag_kind_not_found") {
    afterTagKindRenamedOrDeleted();
  }
  throw error;
}

export async function createTagKind(
  name: string,
  signal?: AbortSignal,
): Promise<TagKind> {
  const created = await request<TagKind>("/api/tag-kinds", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  });
  afterTagKindsChanged();
  return created;
}

export async function renameTagKind(
  id: number,
  name: string,
  signal?: AbortSignal,
): Promise<TagKind> {
  const updated = await request<TagKind>(`/api/tag-kinds/${String(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
    signal,
  }).catch(refreshOnStaleTagKindError);
  afterTagKindRenamedOrDeleted();
  return updated;
}

/** Tags survive kind deletion with no kind assigned. */
export async function deleteTagKind(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(`/api/tag-kinds/${String(id)}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    refreshOnStaleTagKindError(await toRequestFailed(response));
  }
  afterTagKindRenamedOrDeleted();
}

/** Reset shared module state between tests because Vitest reuses modules within a test file. */
export function __resetTagKindsForTest(): void {
  held = undefined;
  generation = 0;
  latestFetch = undefined;
  listeners.clear();
}
