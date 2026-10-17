import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { videoItem } from "../api/libraryItems";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

// ホームの「Not on any shelf」の「See all」が開く `/library?unshelved=1`
// （specs/057-home/ui-design.md「Home」、research.md R-11）。ライブラリは URL の `unshelved=1` を
// 一覧の条件 `unshelved=true` として送る。ゲストも使える。

function video(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: true,
    sizeBytes: 1024 * 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    previewState: "pending",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    tags: [],
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function LocationProbe() {
  return <span data-testid="location">{useLocation().search}</span>;
}

function renderAt(initial: string, audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route
                  path="/library"
                  element={
                    <>
                      <LibraryPage />
                      <LocationProbe />
                    </>
                  }
                />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const fetchMock = vi.fn<typeof fetch>();

function requests(pathname: string): URL[] {
  return fetchMock.mock.calls
    .map((call) => new URL(String(call[0]), "http://localhost"))
    .filter((url) => url.pathname === pathname);
}

describe("棚に無い動画のみのライブラリ", () => {
  beforeEach(() => {
    __resetTagsForTest();
    clearListSnapshot();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    fetchMock.mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
      const path = url.pathname;
      if (path === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (path === "/api/media-folders") return Promise.resolve(json([{}]));
      if (path === "/api/processing") {
        return Promise.resolve(
          json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
        );
      }
      if (path === "/api/tags") return Promise.resolve(json({ items: [] }));
      if (path === "/api/library") {
        const ids = url.searchParams.get("unshelved") === "true" ? [2] : [1, 2];
        return Promise.resolve(
          json({ items: ids.map((id) => videoItem(video(id))), total: ids.length }),
        );
      }
      throw new Error(`unexpected request: ${url.toString()}`);
    });
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it.each<Audience>(["owner", "guest"])(
    "%s: unshelved=1 なら一覧を unshelved=true で読み、URL に残す",
    async (audience) => {
      renderAt("/library?unshelved=1", audience);

      expect(await screen.findByRole("link", { name: "動画 2" })).toBeDefined();
      expect(screen.queryByRole("link", { name: "動画 1" })).toBeNull();
      expect(requests("/api/library").at(-1)?.searchParams.get("unshelved")).toBe("true");
      await waitFor(() =>
        expect(
          new URLSearchParams(screen.getByTestId("location").textContent).get(
            "unshelved",
          ),
        ).toBe("1"),
      );
    },
  );

  it("unshelved が無ければ送らない", async () => {
    renderAt("/library");

    expect(await screen.findByRole("link", { name: "動画 1" })).toBeDefined();
    expect(requests("/api/library").at(-1)?.searchParams.has("unshelved")).toBe(false);
  });
});
