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
let libraryQueries: URLSearchParams[] = [];
let rejectRule = false;

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
    if (path === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (path === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    if (path === "/api/library") {
      libraryQueries.push(url.searchParams);
      return Promise.resolve(json({ items: [videoItem(video(1))], total: 1 }));
    }
    if (path === "/api/smart-folders" && init?.method === "POST") {
      if (rejectRule) {
        return Promise.resolve(
          json({ code: "invalid_smart_rule", message: "rejected" }, 400),
        );
      }
      const body = JSON.parse(String(init.body)) as { name: string };
      created.push(body);
      return Promise.resolve(json({ id: 9, name: body.name }, 201));
    }
    if (path === "/api/smart-folders") return Promise.resolve(json({ smartFolders: [] }));
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

function location(): URLSearchParams {
  return new URLSearchParams(screen.getByTestId("location").textContent);
}

const conditioned =
  "/?minDurationMs=1800000&addedFrom=2024-01-01&addedTo=2024-12-31&watchState=unwatched";

describe("Duration, added date and watch state conditions", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    clearListSnapshot();
    created = [];
    libraryQueries = [];
    rejectRule = false;
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

  it("URL の条件ごとにチップを出してリストに送り、1 つ外すと URL からそれだけ消える", async () => {
    const user = userEvent.setup();
    renderAt(conditioned);
    await screen.findByRole("link", { name: "動画 1" });
    const sent = libraryQueries.at(-1);
    expect(sent?.get("minDurationMs")).toBe("1800000");
    expect(sent?.get("addedFrom")).toBe("2024-01-01");
    expect(sent?.get("addedTo")).toBe("2024-12-31");
    expect(sent?.get("watchState")).toBe("unwatched");

    const chips = within(screen.getByRole("list", { name: "Tags in the filter" }));
    expect(
      chips
        .getAllByRole("button")
        .map((chip) => [chip.getAttribute("aria-label"), chip.textContent]),
    ).toEqual([
      ["Remove the duration filter", "30 min or longer"],
      ["Remove the added-date filter", "Added Jan 1, 2024–Dec 31, 2024"],
      ["Remove the watch filter", "Unwatched"],
    ]);
    expect(screen.getByRole("button", { name: "Filter (3 applied)" })).toBeDefined();

    await user.click(chips.getByRole("button", { name: "Remove the added-date filter" }));
    await waitFor(() => expect(location().has("addedFrom")).toBe(false));
    expect(location().has("addedTo")).toBe(false);
    expect(location().get("minDurationMs")).toBe("1800000");
    expect(location().get("watchState")).toBe("unwatched");
  });

  it("保存ボタンはルールに条件を送り、ダイアログで外したものは送らない", async () => {
    const user = userEvent.setup();
    renderAt(conditioned);
    await screen.findByRole("link", { name: "動画 1" });

    await user.click(screen.getByRole("button", { name: "Save as smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    const row = within(
      within(dialog).getByRole("list", { name: "Set by the assistant" }),
    );
    expect(row.getAllByRole("listitem").map((item) => item.textContent)).toEqual([
      "30 min or longer",
      "Added Jan 1, 2024–Dec 31, 2024",
      "Unwatched",
    ]);
    await user.click(row.getByRole("button", { name: "Remove the watch filter" }));
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "長い{Enter}");

    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]).toEqual({
      name: "長い",
      rule: { minDurationMs: 1_800_000, addedFrom: "2024-01-01", addedTo: "2024-12-31" },
      sort: "addedDesc",
    });
  });

  it("サーバーがルールを拒んだらダイアログを開いたまま条件の誤りを出す", async () => {
    rejectRule = true;
    const user = userEvent.setup();
    renderAt(conditioned);
    await screen.findByRole("link", { name: "動画 1" });
    await user.click(screen.getByRole("button", { name: "Save as smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "長い{Enter}");

    expect(
      await within(dialog).findByText("A duration or date here is out of order"),
    ).toBeDefined();
    expect(screen.getByRole("dialog", { name: "New smart folder" })).toBeDefined();
  });
});
