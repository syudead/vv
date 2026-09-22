import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createMediaFolder,
  deleteMediaFolder,
  listDirectories,
  listMediaFolders,
  startScan,
  transcodeUrl,
  updateMediaFolder,
} from "./client";

describe("playback URLs", () => {
  it("transcode startMsを省略または整数化する", () => {
    expect(transcodeUrl(7)).toBe("/api/videos/7/transcode.mp4");
    expect(transcodeUrl(7, 12_345.4)).toBe("/api/videos/7/transcode.mp4?startMs=12345");
  });
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("settings API client", () => {
  it("sends individual folder mutations with generated contract shapes", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(jsonResponse({ id: 1, path: "/a", version: 1 }))
      .mockResolvedValueOnce(jsonResponse({ id: 1, path: "/b", version: 2 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(
        jsonResponse({ id: 1, state: "running", total: 0, completed: 0, failed: 0 }),
      );
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;

    await listMediaFolders(signal);
    await createMediaFolder("/a", signal);
    await updateMediaFolder(1, "/b", 1, signal);
    await deleteMediaFolder(1, 2, signal);
    await startScan(signal);

    expect(fetch).toHaveBeenNthCalledWith(1, "/api/media-folders", { signal });
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/media-folders",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ path: "/a" }),
        signal,
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/media-folders/1",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ path: "/b", version: 1 }),
        signal,
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/media-folders/1?version=2", {
      method: "DELETE",
      signal,
    });
    expect(fetch).toHaveBeenNthCalledWith(
      5,
      "/api/scans",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({}),
        signal,
      }),
    );
  });

  it("encodes directory paths and forwards cancellation", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(
        jsonResponse({ currentPath: "C:\\Media Files", directories: [] }),
      );
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;

    await listDirectories("C:\\Media Files", signal);

    expect(fetch).toHaveBeenCalledWith("/api/directories?path=C%3A%5CMedia+Files", {
      signal,
    });
  });
});
