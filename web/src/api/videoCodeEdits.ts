import { request } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import { noteSmartFolderChanges } from "./smartFolderChanges";
import { editTargetFields, type VideoEditTargets } from "./videoEdits";

export type VideoCodeEditSummary = components["schemas"]["VideoCodeEditSummary"];
export type VideoCodeEditItem = components["schemas"]["VideoCodeEditItem"];
export type VideoCodeEditResponse = components["schemas"]["VideoCodeEditResponse"];

/** The most distinct videos one code editor takes, as the server's `too_many_videos` limit. */
export const maxCodeEditVideos = 1000;

/** One row per user key of the selection, with its current code; the server resolves smart folders. */
export function summarizeVideoCodeEdits(
  targets: VideoEditTargets,
  signal?: AbortSignal,
): Promise<VideoCodeEditSummary> {
  return request<VideoCodeEditSummary>("/api/video-code-edits/summary", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(editTargetFields(targets)),
    signal,
  });
}

/**
 * Write every item's code in one transaction. The written cards update through the server's
 * `video` events; smart-folder cards and list snapshots are not reached by those, so they are
 * re-read here.
 */
export async function applyVideoCodeEdits(
  items: readonly VideoCodeEditItem[],
  smartFolderIds: readonly number[],
): Promise<VideoCodeEditResponse> {
  const result = await request<VideoCodeEditResponse>("/api/video-code-edits", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ items }),
  });
  clearListSnapshot();
  noteSmartFolderChanges(smartFolderIds);
  return result;
}
