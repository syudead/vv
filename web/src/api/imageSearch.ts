import { request } from "./client";
import type { components } from "./gen/openapi";
import { videoFramePath } from "./personFaces";

export type ImageSearchResult = components["schemas"]["ImageSearchResult"];
export type ImageSearchItem = components["schemas"]["ImageSearchItem"];
export type ImageSearchMatch = components["schemas"]["ImageSearchMatch"];
export type ImageSearchBand = components["schemas"]["ImageSearchBand"];
export type ImageSearchBandId = components["schemas"]["ImageSearchBandId"];
export type ImageSearchMatchFrame = components["schemas"]["ImageSearchMatchFrame"];

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

// A strip slot is related-thumb (160 px) wide; twice that keeps the frame sharp on a 2x screen.
const stripFrameEdge = 320;

/** The picture of one matched frame, cut from the video the match was found in at the strip's size. */
export function matchFramePath(
  match: ImageSearchMatch,
  frame: ImageSearchMatchFrame,
): string {
  return videoFramePath(match.videoId, frame.positionMs, stripFrameEdge);
}
