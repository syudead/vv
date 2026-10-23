import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { videoItem } from "../api/libraryItems";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    virtual: false,
    title: `動画 ${String(id)}`,
    public: true,
    sizeBytes: 1024,
    addedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    fileCreatedAt: "2026-10-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    durationMs: 60_000,
    tags: [],
    ...extra,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function VideoPageProbe() {
  const state = useLocation().state as { from?: string } | null;
  return <p>video page from {state?.from}</p>;
}

function renderAt(initial: string, audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
                <Route path="/videos/7" element={<VideoPageProbe />} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const fetchMock = vi.fn<typeof fetch>();
let created: unknown[] = [];

function install() {
  fetchMock.mockImplementation((input, init) => {
    const path = new URL(String(input), "http://localhost").pathname;
    if (path === "/api/scans/current") return Promise.resolve(json({}, 404));
    if (path === "/api/media-folders") return Promise.resolve(json([{}]));
    if (path === "/api/processing") {
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    }
    if (path === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (path === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    if (path === "/api/smart-folders") return Promise.resolve(json({ smartFolders: [] }));
    if (path === "/api/library") {
      return Promise.resolve(json({ items: [videoItem(video(1))], total: 1 }));
    }
    if (path === "/api/virtual-videos" && init?.method === "POST") {
      const body = JSON.parse(String(init.body)) as { title: string };
      created.push(body);
      return Promise.resolve(json(video(7, { virtual: true, title: body.title }), 201));
    }
    throw new Error(`unexpected request: ${path}`);
  });
}

describe("New virtual video", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    clearListSnapshot();
    created = [];
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    install();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.unstubAllGlobals();
  });

  it("空白だけの題名は送らずに規則を出し、題名を送ると一覧を戻り先にして動画のページを開く", async () => {
    const user = userEvent.setup();
    renderAt("/");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "New virtual video" }));
    const dialog = await screen.findByRole("dialog", { name: "New virtual video" });
    const field = within(dialog).getByRole("textbox", { name: "Title" });
    await user.type(field, "   ");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Enter a title.")).toBeDefined();
    expect(created).toHaveLength(0);

    await user.clear(field);
    await user.type(field, " 京都の記録 {Enter}");
    await waitFor(() => expect(created).toEqual([{ title: "京都の記録" }]));
    expect(await screen.findByText("video page from /")).toBeDefined();
  });

  it("絞り込み中は Save as smart folder の後に置き、ゲストには出さない", async () => {
    renderAt("/?playable=1");
    await screen.findByRole("link", { name: "動画 1" });
    const header = screen
      .getByRole("heading", { name: "Search results" })
      .closest<HTMLElement>("[data-slot=page-header]")!;
    expect(
      within(header)
        .getAllByRole("button")
        .map((button) => button.textContent),
    ).toEqual(["Save as smart folder", "New virtual video"]);
    expect(screen.getAllByRole("button", { name: "New virtual video" })).toHaveLength(1);
    cleanup();

    renderAt("/", "guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: "New virtual video" })).toBeNull();
  });
});
