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
    public: true,
    sizeBytes: 1024 * 1024,
    addedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    fileCreatedAt: "2026-10-01T00:00:00Z",
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
                  path="/"
                  element={
                    <>
                      <LibraryPage />
                      <LocationProbe />
                    </>
                  }
                />
                <Route path="/smart-folders/9" element={<p>Smart folder page</p>} />
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
        json({ items: [{ id: 1, name: "猫", synonyms: [], videoCount: 2 }] }),
      );
    }
    if (path === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    if (path === "/api/library") {
      return Promise.resolve(json({ items: [videoItem(video(1))], total: 1 }));
    }
    if (path === "/api/smart-folders" && init?.method === "POST") {
      const body = JSON.parse(String(init.body)) as { name: string };
      created.push(body);
      return Promise.resolve(json({ id: 9, name: body.name }, 201));
    }
    if (path === "/api/smart-folders") return Promise.resolve(json({ smartFolders: [] }));
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

function location(): string {
  const params = new URLSearchParams(screen.getByTestId("location").textContent);
  if (params.get("sort") === "addedDesc") params.delete("sort");
  const search = params.toString();
  return search === "" ? "" : `?${search}`;
}

describe("Save as smart folder", () => {
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
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    // Sonner actions also use pointer capture.
    Element.prototype.setPointerCapture ??= () => undefined;
    Element.prototype.scrollIntoView ??= () => undefined;
    install();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("タグ 猫 と fav=1 で保存すると、それらを持つ rule を送る（受け入れ条件 5）", async () => {
    const user = userEvent.setup();
    renderAt("/?tag=1&fav=1&sort=titleAsc");
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "Save as smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    expect(
      within(dialog)
        .getByRole("checkbox", { name: "Favorites only" })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(await within(dialog).findByText("猫")).toBeDefined();
    expect(within(dialog).getByRole("combobox", { name: "Sort by" }).textContent).toBe(
      "Title, ascending",
    );

    await user.type(
      within(dialog).getByRole("textbox", { name: "Name" }),
      "タグ: 猫{Enter}",
    );
    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]).toEqual({
      name: "タグ: 猫",
      rule: { favorite: true, tags: [1] },
      sort: "titleAsc",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByText('Created "タグ: 猫"')).toBeDefined();
    expect(location()).toBe("?tag=1&fav=1&sort=titleAsc");

    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByText("Smart folder page")).toBeDefined();
  });

  it("名前が無ければ送らずに名前の規則を出す", async () => {
    const user = userEvent.setup();
    renderAt("/?fav=1");
    await screen.findByRole("link", { name: "動画 1" });
    await user.click(screen.getByRole("button", { name: "Save as smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(created).toHaveLength(0);
  });

  it("ランダムの並びで開くと追加日の新しい順にし、添え書きを出す", async () => {
    const user = userEvent.setup();
    renderAt("/?fav=1&sort=random&seed=5");
    await screen.findByRole("link", { name: "動画 1" });
    await user.click(screen.getByRole("button", { name: "Save as smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    expect(within(dialog).getByRole("combobox", { name: "Sort by" }).textContent).toBe(
      "Date added, newest first",
    );
    expect(
      within(dialog).getByText(
        "Random can't be saved; the folder is sorted by date added",
      ),
    ).toBeDefined();
  });

  it("条件が無いときと、ゲストには出さない", async () => {
    renderAt("/");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: "Save as smart folder" })).toBeNull();
    cleanup();

    renderAt("/?fav=1", "guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: "Save as smart folder" })).toBeNull();
  });
});
