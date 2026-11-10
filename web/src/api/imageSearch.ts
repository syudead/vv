import { request } from "./client";
import type { components } from "./gen/openapi";
import { videoFramePath } from "./personFaces";

export type ImageSearchResult = components["schemas"]["ImageSearchResult"];
export type ImageSearchItem = components["schemas"]["ImageSearchItem"];
export type ImageSearchMatch = components["schemas"]["ImageSearchMatch"];

/**
 * Searches the library by one image or video. The server keeps nothing it was given; aborting the signal ends the
 * search, so the page aborts on a new file, a text search, clearing the image search or leaving.
 */
export function searchByImage(
  file: Blob,
  signal?: AbortSignal,
): Promise<ImageSearchResult> {
  const form = new FormData();
  form.append("file", file, file instanceof File ? file.name : "file");
  return request<ImageSearchResult>("/api/image-search", {
    method: "POST",
    body: form,
    signal,
  });
}

/** The matched frame, cut from the video the match was found in. */
export function matchFramePath(match: ImageSearchMatch): string {
  return videoFramePath(match.videoId, match.positionMs);
}
