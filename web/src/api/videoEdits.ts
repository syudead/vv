import { request } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { smartFolderField, afterSmartFolderMembersChanged } from "./smartFolderChanges";
import { afterBulkTagEdit, rethrowStaleTagError, type TagRef } from "./tags";
import { nextVideoTagsSequence } from "./videoTagsEvents";
import { markVisibilityUncertain } from "./visibility";

export type VideoEditSummary = components["schemas"]["VideoEditSummary"];
export type VideoEditRequest = components["schemas"]["VideoEditRequest"];
export type VideoEditResponse = components["schemas"]["VideoEditResponse"];
export type VideoCreditEdit = components["schemas"]["VideoCreditEdit"];
export type VideoDateEdit = components["schemas"]["VideoDateEdit"];

/** The selection a bulk edit reads and writes; the server adds each smart folder's current members. */
export interface VideoEditTargets {
  videoIds: readonly number[];
  smartFolderIds: readonly number[];
}

/** The request fields of a selection, without `smartFolderIds` when there are none. */
export function editTargetFields(targets: VideoEditTargets) {
  return {
    videoIds: Array.from(targets.videoIds),
    ...smartFolderField(targets.smartFolderIds),
  };
}

export function summarizeVideoEdits(
  targets: VideoEditTargets,
  signal?: AbortSignal,
): Promise<VideoEditSummary> {
  return request<VideoEditSummary>("/api/video-edits/summary", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(editTargetFields(targets)),
    signal,
  });
}

export type VideoEditChanges = Omit<VideoEditRequest, "videoIds" | "smartFolderIds">;

/**
 * Apply one bulk edit atomically, then bring open lists up to date: tags through the shared tag
 * overlay, visibility by refreshing the explicit videos, and smart-folder members by re-reading.
 * Work information reaches open pages through the server's video events.
 */
export async function applyVideoEdits(
  targets: VideoEditTargets,
  changes: VideoEditChanges,
  removedTags: readonly TagRef[],
  signal?: AbortSignal,
): Promise<VideoEditResponse> {
  const sequence = nextVideoTagsSequence();
  const result = await request<VideoEditResponse>("/api/video-edits", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...editTargetFields(targets), ...changes }),
    signal,
  }).catch(rethrowStaleTagError);
  const created = changes.tags?.add?.some((tag) => tag.name !== undefined) ?? false;
  afterBulkTagEdit(
    targets.videoIds,
    { added: result.tags, removed: removedTags, created },
    sequence,
  );
  if (changes.public !== undefined) markVisibilityUncertain(targets.videoIds);
  clearListSnapshot();
  afterSmartFolderMembersChanged(targets.smartFolderIds);
  return result;
}
