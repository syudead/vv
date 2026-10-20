import { request, type Video } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";
import {
  afterSmartFolderMembersChanged,
  smartFolderField,
  type VideoTargetOptions,
} from "./smartFolderChanges";

export type WorkInfo = components["schemas"]["WorkInfo"];
export type WorkInfoInput = components["schemas"]["WorkInfoInput"];
export type WorkDateKind = components["schemas"]["WorkDateKind"];
export type WorkPersonRole = components["schemas"]["WorkPersonRole"];
export type WorkOrganizationRole = components["schemas"]["WorkOrganizationRole"];
export type VideoDetailsInput = components["schemas"]["VideoDetailsInput"];
export type VideoCreditEdit = components["schemas"]["VideoCreditEdit"];
export type VideoCreditsResponse = components["schemas"]["VideoCreditsResponse"];

export const workDateKinds: readonly WorkDateKind[] = [
  "release",
  "recorded",
  "published",
];

export const workPersonRoles: readonly WorkPersonRole[] = [
  "performer",
  "director",
  "photographer",
  "streamer",
];

export const workOrganizationRoles: readonly WorkOrganizationRole[] = ["maker", "label"];

/** Save display name and metadata atomically; invalidate detached list snapshots whose titles cannot receive live updates. */
export async function setVideoDetails(
  id: number,
  body: VideoDetailsInput,
  signal?: AbortSignal,
): Promise<Video> {
  const video = await request<Video>(`/api/videos/${String(id)}/details`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  clearListSnapshot();
  return video;
}

/** Apply role edits to the selection atomically; work-info changes reach open lists through video events. */
export async function editVideoCredits(
  videoIds: readonly number[],
  edits: readonly VideoCreditEdit[],
  { smartFolderIds = [], signal }: VideoTargetOptions = {},
): Promise<VideoCreditsResponse> {
  const result = await request<VideoCreditsResponse>("/api/video-credits", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      videoIds: Array.from(videoIds),
      ...smartFolderField(smartFolderIds),
      edits,
    }),
    signal,
  });
  clearListSnapshot();
  afterSmartFolderMembersChanged(smartFolderIds);
  return result;
}
