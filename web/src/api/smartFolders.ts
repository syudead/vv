import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { dropSnapshotAfter } from "./groups";

// Library items carry summaries; fetch card values separately for visible folders.
//

export type SmartRule = components["schemas"]["SmartRule"];
export type SmartRuleRef = components["schemas"]["SmartRuleRef"];
export type SmartFolderSummary = components["schemas"]["SmartFolderSummary"];
export type LibrarySmartFolder = components["schemas"]["LibrarySmartFolder"];
export type SmartFolderSettings = components["schemas"]["SmartFolderSettings"];
export type SmartFolderItem = components["schemas"]["SmartFolderItem"];
export type SmartFolderDetail = components["schemas"]["SmartFolderDetail"];
export type SmartFolderItemPage = components["schemas"]["SmartFolderItemPage"];
export type SmartFolderCreate = components["schemas"]["SmartFolderCreate"];
export type SmartFolderChange = components["schemas"]["SmartFolderChange"];

export const MAX_SMART_FOLDER_CARDS = 60;

const jsonHeaders = { "Content-Type": "application/json" };

function smartFolderPath(id: number): string {
  return `/api/smart-folders/${String(id)}`;
}

export async function listSmartFolders(
  signal?: AbortSignal,
): Promise<SmartFolderSummary[]> {
  const list = await request<components["schemas"]["SmartFolderList"]>(
    "/api/smart-folders",
    {
      signal,
    },
  );
  return list.smartFolders;
}

export function readSmartFolderPage(
  id: number,
  signal?: AbortSignal,
  limit?: number,
): Promise<SmartFolderDetail> {
  const query = limit === undefined ? "" : `?limit=${String(limit)}`;
  return request<SmartFolderDetail>(`${smartFolderPath(id)}${query}`, { signal });
}

/** Follow nextCursor without interpreting it. */
export function listSmartFolderItems(
  id: number,
  cursor: string,
  signal?: AbortSignal,
  limit?: number,
): Promise<SmartFolderItemPage> {
  const query = new URLSearchParams({ cursor });
  if (limit !== undefined) query.set("limit", String(limit));
  return request<SmartFolderItemPage>(
    `${smartFolderPath(id)}/items?${query.toString()}`,
    {
      signal,
    },
  );
}

/** Preserve requested ID order, omitting unavailable folders; an empty input makes no request. */
export async function readSmartFolderCards(
  ids: readonly number[],
  signal?: AbortSignal,
): Promise<LibrarySmartFolder[]> {
  if (ids.length === 0) return [];
  // Numeric IDs allow literal comma separators: form style with explode false.
  const answer = await request<components["schemas"]["SmartFolderCards"]>(
    `/api/smart-folders/cards?ids=${ids.map(String).join(",")}`,
    { signal },
  );
  return answer.cards;
}

export function createSmartFolder(
  folder: SmartFolderCreate,
  signal?: AbortSignal,
): Promise<SmartFolderSummary> {
  return dropSnapshotAfter(
    request<SmartFolderSummary>("/api/smart-folders", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(folder),
      signal,
    }),
  );
}

/** Patch supplied fields only; null parentId moves to the root, and descendant moves fail with smart_folder_cycle. */
export function changeSmartFolder(
  id: number,
  change: SmartFolderChange,
  signal?: AbortSignal,
): Promise<SmartFolderSummary> {
  return dropSnapshotAfter(
    request<SmartFolderSummary>(smartFolderPath(id), {
      method: "PATCH",
      headers: jsonHeaders,
      body: JSON.stringify(change),
      signal,
    }),
  );
}

/** Delete the folder subtree without changing videos. */
export function deleteSmartFolder(id: number, signal?: AbortSignal): Promise<void> {
  return dropSnapshotAfter(
    (async () => {
      const response = await apiFetch(smartFolderPath(id), { method: "DELETE", signal });
      if (!response.ok) throw await toRequestFailed(response);
    })(),
  );
}
