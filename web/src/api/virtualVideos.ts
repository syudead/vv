import { request, type Video } from "./client";
import { clearListSnapshot } from "./listSnapshot";

/**
 * Creates a video with no file from a title, a link or both; an empty field is left out. Without a link a blank title
 * answers 400 with reason invalid_title, and a link that is not http or https answers 400 without a reason. A link
 * starts a fetch that fills the empty fields after the response.
 */
export async function createVirtualVideo(
  { title, url }: { title: string; url?: string },
  signal?: AbortSignal,
): Promise<Video> {
  const created = await request<Video>("/api/virtual-videos", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(url ? (title ? { title, url } : { url }) : { title }),
    signal,
  });
  // A held list would hide the new card when the owner returns to it.
  clearListSnapshot();
  return created;
}
