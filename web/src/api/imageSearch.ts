import { request } from "./client";
import type { components } from "./gen/openapi";
import { videoFramePath } from "./personFaces";
import { uploadRequest } from "./upload";

export type ImageSearchResult = components["schemas"]["ImageSearchResult"];
export type ImageSearchItem = components["schemas"]["ImageSearchItem"];
export type ImageSearchMatch = components["schemas"]["ImageSearchMatch"];
export type ImageSearchBand = components["schemas"]["ImageSearchBand"];
export type ImageSearchBandId = components["schemas"]["ImageSearchBandId"];
export type ImageSearchMatchFrame = components["schemas"]["ImageSearchMatchFrame"];

const searchPath = "/api/image-search";

/**
 * Searches the library by one image or video. The server keeps nothing it was given; aborting the signal ends the
 * search, so the page aborts on Cancel, a new file, a text search or leaving. With `onUpload` the file is sent
 * with progress, called with the share sent; without it the search is one plain request.
 */
export function searchByImage(
  file: Blob,
  signal?: AbortSignal,
  onUpload?: (sent: number) => void,
): Promise<ImageSearchResult> {
  const form = new FormData();
  form.append("file", file, file instanceof File ? file.name : "file");
  if (onUpload !== undefined) {
    return uploadRequest<ImageSearchResult>(searchPath, form, {
      signal,
      onProgress: onUpload,
    });
  }
  return request<ImageSearchResult>(searchPath, { method: "POST", body: form, signal });
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
