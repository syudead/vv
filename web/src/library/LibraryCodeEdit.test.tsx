import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

function video(id: number, code: string | undefined): Video {
  return {
    id,
    virtual: false,
    title: `動画 ${String(id)}`,
    fileTitle: `CODE-${String(id)}`,
    contentVersion: `v${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    fileModifiedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    durationMs: 60_000,
    previewState: "pending",
    tags: [],
    cardCredits: { code, performers: [] },
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Video 1 has no code until the apply writes its file name; the server holds the current cards. */
function install() {
  const videos = new Map([[1, video(1, undefined)]]);
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
    if (url.pathname === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/library") {
      const items = [...videos.values()].map((item) => ({ kind: "video", video: item }));
      return Promise.resolve(json({ items, total: items.length }));
    }
    if (url.pathname === "/api/videos/1" && method === "GET") {
      return Promise.resolve(json(videos.get(1)));
    }
    if (url.pathname === "/api/video-code-edits/summary" && method === "POST") {
      return Promise.resolve(json({ rows: [{ video: videos.get(1) }] }));
    }
    if (url.pathname === "/api/video-code-edits" && method === "POST") {
      videos.set(1, video(1, "CODE-1"));
      return Promise.resolve(json({ applied: 1 }));
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

function renderLibrary() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  installFakeEventSource();
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("Edit codes in the library", () => {
  it("shows the written code on the card once its video event arrives, keeping the selection", async () => {
    install();
    const user = userEvent.setup();
    renderLibrary();
    const card = (await screen.findByRole("link", { name: "動画 1" })).closest("article");
    if (card === null) throw new Error("card without article");
    expect(within(card).queryByText("CODE-1")).toBeNull();

    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("button", { name: "Edit codes" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit codes" });
    await user.click(within(dialog).getByRole("button", { name: "Apply to 1 video" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await emitServerEvent("video", { id: 1 });

    expect(await within(card).findByText("CODE-1")).toBeDefined();
    expect(screen.getByText("1 video selected")).toBeDefined();
  });
});
