import { t, type UiText } from "../i18n";
import {
  apiFetch,
  type GroupDetail,
  type LibraryGroup,
  MAX_PAGE_SIZE,
  request,
  RequestFailed,
  toRequestFailed,
} from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { refreshTags } from "./tags";

// client.getGroup refreshes list cards; group pages use readGroupPage and listGroupItems with trail context.

export type GroupItem = components["schemas"]["GroupItem"];
export type GroupRef = components["schemas"]["GroupRef"];
export type GroupItemPage = components["schemas"]["GroupItemPage"];
export type GroupListPage = components["schemas"]["GroupListPage"];
export type GroupTagResult = components["schemas"]["GroupTagResult"];

const jsonHeaders = { "Content-Type": "application/json" };

function groupPath(id: number): string {
  return `/api/groups/${String(id)}`;
}

/** Send trail IDs outermost first; the server returns only the still-valid parent-child chain. */
export function readGroupPage(
  id: number,
  trail: readonly number[],
  signal?: AbortSignal,
  searchQuery = "",
): Promise<GroupDetail> {
  // Numeric IDs allow literal comma separators: form style with explode false.
  const params = new URLSearchParams();
  if (trail.length > 0) params.set("trail", trail.map(String).join(","));
  if (searchQuery !== "") params.set("query", searchQuery);
  const query = params.size === 0 ? "" : `?${params.toString()}`;
  return request<GroupDetail>(`${groupPath(id)}${query}`, { signal });
}

/** Follow nextCursor opaquely; omitted cursor starts at the first page. */
export function listGroupItems(
  id: number,
  cursor: string | undefined,
  signal?: AbortSignal,
  limit?: number,
  searchQuery = "",
): Promise<GroupItemPage> {
  const query = new URLSearchParams();
  if (searchQuery !== "") query.set("query", searchQuery);
  if (cursor !== undefined) query.set("cursor", cursor);
  if (limit !== undefined) query.set("limit", String(limit));
  const search = query.size === 0 ? "" : `?${query.toString()}`;
  return request<GroupItemPage>(`${groupPath(id)}/items${search}`, { signal });
}

const directItemPageSize = 200;

/** Search direct items only, not descendants, because removal requires the group's own item ID. */
export async function findVideoItem(
  id: number,
  videoId: number,
  signal?: AbortSignal,
): Promise<number | undefined> {
  let cursor: string | undefined;
  do {
    const page = await listGroupItems(id, cursor, signal, directItemPageSize);
    const item = page.items.find(
      (each) => each.kind === "video" && each.video?.id === videoId,
    );
    if (item !== undefined) return item.id;
    cursor = page.nextCursor;
  } while (cursor !== undefined);
  return undefined;
}

/** Append videos first, then groups, preserving each supplied order. */
export interface GroupMembers {
  videoIds?: readonly number[];
  groupIds?: readonly number[];
}

function membersBody(members: GroupMembers): { videoIds: number[]; groupIds: number[] } {
  return {
    videoIds: Array.from(members.videoIds ?? []),
    groupIds: Array.from(members.groupIds ?? []),
  };
}

export interface ListGroupsParams {
  /** The server trims and matches case-insensitively. */
  q?: string;
  /** Opaque nextCursor from the previous response. */
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
}

export function listGroups({
  q,
  cursor,
  limit,
  signal,
}: ListGroupsParams = {}): Promise<GroupListPage> {
  const params = new URLSearchParams();
  if (q !== undefined && q.trim() !== "") params.set("q", q);
  if (cursor !== undefined) params.set("cursor", cursor);
  if (limit !== undefined) params.set("limit", String(limit));
  const query = params.toString();
  return request<GroupListPage>(query === "" ? "/api/groups" : `/api/groups?${query}`, {
    signal,
  });
}

/** Read all pages without a page-count cap; reject repeated cursors instead of returning an incomplete list. */
export async function listAllGroups(signal?: AbortSignal): Promise<LibraryGroup[]> {
  const groups: LibraryGroup[] = [];
  const seen = new Set<string>();
  let cursor: string | undefined;
  do {
    const answer = await listGroups({ cursor, limit: MAX_PAGE_SIZE, signal });
    groups.push(...answer.groups);
    cursor = answer.nextCursor;
    if (cursor !== undefined && seen.has(cursor)) {
      throw new Error("The group list returned the same cursor twice.");
    }
    if (cursor !== undefined) seen.add(cursor);
  } while (cursor !== undefined);
  return groups;
}

/** Creation is atomic; a missing video or group returns group_target_not_found. */
export function createGroup(
  name: string,
  members: GroupMembers,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  return dropSnapshotAfter(
    request<LibraryGroup>("/api/groups", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ name, ...membersBody(members) }),
      signal,
    }),
  );
}

/** A valid undoToken includes renaming in creation undo; invalid tokens do not prevent renaming. */
export async function renameGroup(
  id: number,
  name: string,
  { undoToken, signal }: { undoToken?: string; signal?: AbortSignal } = {},
): Promise<LibraryGroup> {
  const group = await request<LibraryGroup>(groupPath(id), {
    method: "PATCH",
    headers: jsonHeaders,
    body: JSON.stringify(undoToken === undefined ? { name } : { name, undoToken }),
    signal,
  });
  clearListSnapshot();
  return group;
}

/** Dissolving preserves child groups and videos. */
export async function dissolveGroup(id: number, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(groupPath(id), { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
  clearListSnapshot();
}

/** Skip existing items; missing targets or cycles reject the entire addition. */
export function addToGroup(
  id: number,
  members: GroupMembers,
  signal?: AbortSignal,
): Promise<LibraryGroup> {
  return dropSnapshotAfter(
    request<LibraryGroup>(`${groupPath(id)}/items`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(membersBody(members)),
      signal,
    }),
  );
}

/** Invalidate snapshots after mutations and 404s: detached lists may retain moved or missing items. Shared with smart-folder mutations. */
export async function dropSnapshotAfter<T>(sent: Promise<T>): Promise<T> {
  try {
    const value = await sent;
    clearListSnapshot();
    return value;
  } catch (failure) {
    if (failure instanceof RequestFailed && failure.status === 404) clearListSnapshot();
    throw failure;
  }
}

/** Remove item IDs, not video/group IDs. Return null when removing the final item dissolves the group with a 204 response. */
export async function removeFromGroup(
  id: number,
  itemIds: readonly number[],
  signal?: AbortSignal,
): Promise<LibraryGroup | null> {
  const response = await apiFetch(`${groupPath(id)}/items/remove`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ itemIds: Array.from(itemIds) }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  const group =
    response.status === 204 ? null : ((await response.json()) as LibraryGroup);
  clearListSnapshot();
  return group;
}

/** Null moves to the front; invalidate snapshots because order determines previews and initial playback. */
export async function moveGroupItem(
  id: number,
  itemId: number,
  after: number | null,
  signal?: AbortSignal,
): Promise<void> {
  const response = await apiFetch(`${groupPath(id)}/items/${String(itemId)}/move`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(after === null ? {} : { after }),
    signal,
  });
  if (!response.ok) throw await toRequestFailed(response);
  clearListSnapshot();
}

/** Tag every descendant video and dissolve the group atomically; refresh shared tags and invalidate snapshots, including on 404. */
export async function tagGroup(
  id: number,
  signal?: AbortSignal,
): Promise<GroupTagResult> {
  const result = await dropSnapshotAfter(
    request<GroupTagResult>(`${groupPath(id)}/tag`, { method: "POST", signal }),
  );
  refreshTags().catch(() => undefined);
  return result;
}

export function taggedMessage(result: GroupTagResult): UiText {
  return t.groups.tagged(result.tag.name, result.created);
}
