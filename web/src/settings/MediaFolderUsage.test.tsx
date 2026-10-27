import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { MediaFolder, MediaFolderUsage as Usage } from "../api/client";
import MediaFolderUsage from "./MediaFolderUsage";

const GB = 1024 ** 3;

function folder(version: number): MediaFolder {
  return { id: 3, path: "/media/a", version, createdAt: "", updatedAt: "" };
}

/** Answers each usage request with the next response and records the requested URLs. */
function serve(...responses: (Usage | number)[]) {
  const urls: string[] = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    urls.push(String(input));
    const next = responses.shift() ?? 500;
    const status = typeof next === "number" ? next : 200;
    const body = typeof next === "number" ? { code: "internal", message: "" } : next;
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  return urls;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("MediaFolderUsage", () => {
  it("shows the free and total disk space with the folder's video size", async () => {
    const urls = serve({
      videoBytes: 340 * GB,
      disk: { freeBytes: 120 * GB, totalBytes: 2048 * GB },
    });
    render(<MediaFolderUsage folder={folder(1)} />);

    expect(await screen.findByText("120 GB free")).toBeDefined();
    expect(screen.getByText("120 GB free").closest("p")?.textContent).toBe(
      "120 GB free of 2.0 TB · Videos 340 GB",
    );
    expect(urls).toEqual(["/api/media-folders/3/usage"]);
  });

  it("marks the disk space unavailable when the volume or the request fails", async () => {
    serve({ videoBytes: 0 }, 500);
    const { unmount } = render(<MediaFolderUsage folder={folder(1)} />);
    expect((await screen.findByText(/Disk space unavailable/)).textContent).toBe(
      "Disk space unavailable · Videos 0 B",
    );
    unmount();

    render(<MediaFolderUsage folder={folder(1)} />);
    expect((await screen.findByText(/Disk space unavailable/)).textContent).toBe(
      "Disk space unavailable",
    );
  });

  it("reads the usage again for a changed folder", async () => {
    const urls = serve(
      { videoBytes: 0, disk: { freeBytes: GB, totalBytes: 2 * GB } },
      { videoBytes: 0, disk: { freeBytes: 5 * GB, totalBytes: 9 * GB } },
    );
    const { rerender } = render(<MediaFolderUsage folder={folder(1)} />);
    await screen.findByText("1.0 GB free");

    rerender(<MediaFolderUsage folder={folder(2)} />);
    expect(await screen.findByText("5.0 GB free")).toBeDefined();
    expect(screen.queryByText("1.0 GB free")).toBeNull();
    expect(urls).toHaveLength(2);
  });
});
