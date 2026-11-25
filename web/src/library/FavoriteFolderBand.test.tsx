import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SmartFolderSummary, Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import LibraryPage from "./LibraryPage";

function video(id: number): Video {
  return {
    id,
    virtual: false,
    title: `動画 ${String(id)}`,
    public: true,
    sizeBytes: 1024 * 1024,
    addedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    fileCreatedAt: "2026-10-01T00:00:00Z",
    fileModifiedAt: "2026-10-01T00:00:00Z",
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

const cat = {
  id: 3,
  name: "猫",
  manual: true,
  fromFolder: false,
  tentative: false,
  kind: null,
  videoCount: 1,
  synonyms: [],
};

/** GET /api/smart-folders answers the folders in name order, as the server does. */
let folders: SmartFolderSummary[] = [];
let folderReads = 0;
let failFolderReads = false;

const fetchMock = vi.fn<typeof fetch>((input) => {
  const url = new URL(String(input), "http://localhost");
  switch (url.pathname) {
    case "/api/smart-folders":
      folderReads += 1;
      return Promise.resolve(
        failFolderReads
          ? json({ code: "internal", message: "broken" }, 500)
          : json({ smartFolders: folders }),
      );
    case "/api/library":
      return Promise.resolve(
        json({ items: [{ kind: "video", video: video(1) }], total: 1 }),
      );
    case "/api/scans/current":
      return Promise.resolve(json({}, 404));
    case "/api/media-folders":
      return Promise.resolve(json([{}]));
    case "/api/processing":
      return Promise.resolve(
        json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
      );
    case "/api/tags":
      return Promise.resolve(json({ items: [cat] }));
    case "/api/tag-kinds":
      return Promise.resolve(json({ items: [] }));
    case "/api/people":
    case "/api/organizations":
      return Promise.resolve(json({ items: [], total: 0, totalAll: 0 }));
  }
  throw new Error(`unexpected request: ${url.toString()}`);
});

function renderHome(initial = "/", audience: Audience = "owner") {
  render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
                <Route path="/smart-folders" element={<p>Smart folder list</p>} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function band(): HTMLElement | null {
  return (
    screen
      .queryByRole("heading", { name: "Favorite smart folders" })
      ?.closest<HTMLElement>("section") ?? null
  );
}

function bandNames(): string[] {
  const section = band();
  if (section === null) return [];
  return within(section)
    .getAllByRole("article")
    .map((card) => within(card).getByRole("heading").textContent ?? "");
}

/** The list has loaded once its video card is drawn, so the band's read has been sent. */
async function listLoaded() {
  await screen.findByRole("checkbox", { name: 'Select "動画 1"' });
}

beforeEach(() => {
  __resetTagsForTest();
  __resetTagKindsForTest();
  clearListSnapshot();
  localStorage.clear();
  folders = [
    { id: 1, name: "A", favorite: true },
    { id: 2, name: "B", favorite: false },
    { id: 3, name: "C", favorite: true },
    { id: 4, name: "C-1", parentId: 3, favorite: true },
  ];
  folderReads = 0;
  failFolderReads = false;
  vi.stubGlobal("fetch", fetchMock);
  // Card values stay unread; the band's own read is the contract here.
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  installFakeEventSource();
});

afterEach(() => {
  cleanup();
  fetchMock.mockClear();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("The band of favorite smart folders on the home", () => {
  it("holds the favorites at any depth in the list's order, counts them and links to every folder", async () => {
    const user = userEvent.setup();
    renderHome();

    await waitFor(() => expect(bandNames()).toEqual(["A", "C", "C-1"]));
    const section = band() as HTMLElement;
    expect(within(section).getByText("3 favorites")).toBeTruthy();

    await user.click(within(section).getByRole("link", { name: "All smart folders" }));
    expect(await screen.findByText("Smart folder list")).toBeTruthy();
  });

  it("is not drawn without a favorite folder", async () => {
    folders = folders.map((folder) => ({ ...folder, favorite: false }));
    renderHome();
    await listLoaded();
    await waitFor(() => expect(folderReads).toBe(1));

    expect(band()).toBeNull();
  });

  it.each([
    ["a search", "/?q=cat"],
    ["a tag filter", "/?tag=3"],
  ])("is neither read nor drawn under %s", async (_, path) => {
    renderHome(path);
    await listLoaded();

    expect(band()).toBeNull();
    expect(folderReads).toBe(0);
  });

  it("stays under the favorites-only filter", async () => {
    renderHome("/?fav=1");

    await waitFor(() => expect(bandNames()).toEqual(["A", "C", "C-1"]));
  });

  it("reads the list again on a smartFolder event, so favorites changed elsewhere join and leave", async () => {
    renderHome();
    await waitFor(() => expect(bandNames()).toEqual(["A", "C", "C-1"]));

    folders = [
      { id: 1, name: "A", favorite: false },
      { id: 2, name: "B", favorite: true },
      { id: 3, name: "C", favorite: true },
    ];
    await emitServerEvent("smartFolder", { ids: [1, 2, 3, 4] });
    await waitFor(() => expect(bandNames()).toEqual(["B", "C"]));

    folders = folders.map((folder) => ({ ...folder, favorite: false }));
    await emitServerEvent("smartFolder", { ids: [1, 2, 3] });
    await waitFor(() => expect(band()).toBeNull());
    await listLoaded();
  });

  it("shows the failure with Retry under its heading while the list still loads", async () => {
    const user = userEvent.setup();
    failFolderReads = true;
    renderHome();
    await listLoaded();

    const failure = await screen.findByText(/^Couldn't load the smart folders: /);
    expect(band()?.contains(failure)).toBe(true);
    failFolderReads = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(bandNames()).toEqual(["A", "C", "C-1"]));
  });

  it("runs a Shift range from a band card into the list in display order", async () => {
    const user = userEvent.setup();
    renderHome();
    await waitFor(() => expect(bandNames()).toEqual(["A", "C", "C-1"]));
    await listLoaded();

    await user.click(
      screen.getByRole("checkbox", { name: 'Select the smart folder "A"' }),
    );
    await user.keyboard("{Shift>}");
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.keyboard("{/Shift}");

    expect(screen.getByText("1 video and 3 smart folders selected")).toBeTruthy();
  });

  it("drops a selected folder from the selection when it leaves the band", async () => {
    const user = userEvent.setup();
    renderHome();
    await waitFor(() => expect(bandNames()).toEqual(["A", "C", "C-1"]));
    await user.click(
      screen.getByRole("checkbox", { name: 'Select the smart folder "A"' }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: 'Select the smart folder "C"' }),
    );
    expect(screen.getByText("2 smart folders selected")).toBeTruthy();

    folders = folders.map((folder) =>
      folder.id === 1 ? { ...folder, favorite: false } : folder,
    );
    await emitServerEvent("smartFolder", { ids: [1] });
    await waitFor(() => expect(bandNames()).toEqual(["C", "C-1"]));

    expect(screen.getByText("1 smart folder selected")).toBeTruthy();
  });

  it("is neither read nor drawn for a guest", async () => {
    renderHome("/", "guest");
    await screen.findByRole("link", { name: /動画 1/ });

    expect(band()).toBeNull();
    expect(folderReads).toBe(0);
  });
});
