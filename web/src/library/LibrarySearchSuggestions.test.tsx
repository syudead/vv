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

function video(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
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

function credit(id: number, name: string) {
  return { id, name, aliases: [], externalIds: [], videoCount: 1 };
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
                  path="/"
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

function install() {
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
    if (path === "/api/tags") {
      return Promise.resolve(
        json({ items: [{ id: 2, name: "Alps", synonyms: [], videoCount: 1 }] }),
      );
    }
    if (path === "/api/tag-kinds") {
      return Promise.resolve(
        json({ items: [{ id: 4, name: "Album", tagCount: 1, createdAt: "" }] }),
      );
    }
    if (path === "/api/people") {
      const q = url.searchParams.get("q");
      const items = q === "Al" ? [credit(1, "Alice")] : [];
      return Promise.resolve(json({ items, total: items.length, totalAll: 1 }));
    }
    if (path === "/api/organizations") {
      return Promise.resolve(json({ items: [], total: 0, totalAll: 0 }));
    }
    if (path === "/api/people/1") return Promise.resolve(json(credit(1, "Alice")));
    if (path === "/api/library") {
      return Promise.resolve(json({ items: [videoItem(video(1))], total: 1 }));
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

function requests(pathname: string): URL[] {
  return fetchMock.mock.calls
    .map((call) => new URL(String(call[0]), "http://localhost"))
    .filter((url) => url.pathname === pathname);
}

function location(): string {
  const params = new URLSearchParams(screen.getByTestId("location").textContent);
  if (params.get("sort") === "addedDesc") params.delete("sort");
  const search = params.toString();
  return search === "" ? "" : `?${search}`;
}

describe("search suggestions on the library", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    clearListSnapshot();
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
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("picking a person applies the person filter, clears the text and shows the chip", async () => {
    const user = userEvent.setup();
    renderAt("/?sort=titleAsc");
    await screen.findByRole("link", { name: "動画 1" });
    const box = screen.getByRole("combobox", { name: "Search videos" });

    await user.type(box, "Al");
    const list = await screen.findByRole("listbox", { name: "Search suggestions" });
    await within(list).findByRole("option", { name: "Alice" });
    expect(
      within(list)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Search for “Al”", "Alice", "Kind: Album", "Alps"]);
    expect(within(list).queryByText("Organizations")).toBeNull();

    await user.click(within(list).getByRole("option", { name: "Alice" }));
    await waitFor(() => expect(location()).toBe("?sort=titleAsc&person=1"));
    expect((box as HTMLInputElement).value).toBe("");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(
      await screen.findByRole("button", { name: "Remove the filter for Alice" }),
    ).toBeDefined();
    await waitFor(() => {
      const last = requests("/api/library").at(-1);
      expect(last?.searchParams.get("person")).toBe("1");
      expect(last?.searchParams.has("query")).toBe(false);
    });
  });

  it("choosing a tag kind with the keyboard sets tagKind and drops a committed search", async () => {
    const user = userEvent.setup();
    renderAt("/");
    await screen.findByRole("link", { name: "動画 1" });
    const box = screen.getByRole("combobox", { name: "Search videos" });

    await user.type(box, "Al{Enter}");
    await waitFor(() => expect(location()).toBe("?q=Al"));
    // Enter closed the list; ArrowDown reopens it on "Search for".
    await user.keyboard("{ArrowDown}");
    await screen.findByRole("option", { name: "Kind: Album" });
    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    await waitFor(() => expect(location()).toBe("?tagKind=4"));
    expect((box as HTMLInputElement).value).toBe("");
  });

  it("guests get a plain search field without suggestion requests", async () => {
    const user = userEvent.setup();
    renderAt("/", "guest");
    await screen.findByRole("link", { name: "動画 1" });

    await user.type(
      screen.getByRole("searchbox", { name: "Search videos" }),
      "Al{Enter}",
    );
    await waitFor(() => expect(location()).toBe("?q=Al"));
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(requests("/api/people")).toHaveLength(0);
  });
});
