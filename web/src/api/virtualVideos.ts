import { request, type Video } from "./client";
import { clearListSnapshot } from "./listSnapshot";

/** Creates a video with no file; a blank title answers 400 invalid_title. */
export async function createVirtualVideo(
  title: string,
  signal?: AbortSignal,
): Promise<Video> {
  const created = await request<Video>("/api/virtual-videos", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title }),
    signal,
  });
  // A held list would hide the new card when the owner returns to it.
  clearListSnapshot();
  return created;
}
