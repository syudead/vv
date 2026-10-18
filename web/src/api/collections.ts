import { request } from "./client";
import type { components } from "./gen/openapi";
import { dropSnapshotAfter } from "./groups";

export type CollectionsDeleteResponse =
  components["schemas"]["CollectionsDeleteResponse"];

/** Dissolve groups and delete smart-folder trees in one request; missing IDs are ignored and videos stay. */
export function deleteCollections(
  groupIds: readonly number[],
  smartFolderIds: readonly number[],
  signal?: AbortSignal,
): Promise<CollectionsDeleteResponse> {
  return dropSnapshotAfter(
    request<CollectionsDeleteResponse>("/api/collections/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ groupIds, smartFolderIds }),
      signal,
    }),
  );
}
