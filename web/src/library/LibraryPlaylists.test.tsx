import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024 * 1024 * id,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    durationMs: 60_000 * id,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "pending",
    tags: [],
    favorite: false,
    ...extra,
  };
}

const memberIds = [101, 102, 103];
const seriesFolder = { rootId: 3, path: "series" };

function seriesGroup(extra: Partial<LibraryGroup> = {}): LibraryGroup {
  return {
    id: 7,
    folder: seriesFolder,
    name: "series",
    videoCount: 3,
    watchedCount: 0,
    watchState: "unwatched",
    durationMs: 3 * 60_000,
    sizeBytes: 3 * 1024 * 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: memberIds.map((id) => ({
      videoId: id,
      thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    })),
    openVideoId: 101,
    videoIds: memberIds,
    tags: [],
    favorite: false,
    ...extra,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

interface AddBody {
  videoIds: number[];
  groupIds: number[];
}

const fetchMock = vi.fn<typeof fetch>();

function install(items: LibraryItem[]): { adds: AddBody[] } {
  const server = { adds: [] as AddBody[] };
  fetchMock.mockImplementation((input, init) => {
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
      return Promise.resolve(json({ items, total: items.length }));
    }
    if (url.pathname === "/api/playlists" && method === "GET") {
      return Promise.resolve(
        json({
          items: [
            { id: 5, name: "Tonight", itemCount: 0, updatedAt: "2026-10-01T00:00:00Z" },
          ],
          total: 1,
          version: "1",
        }),
      );
    }
    if (url.pathname === "/api/playlists/5/items" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as AddBody;
      server.adds.push(body);
      return Promise.resolve(
        json({ added: body.videoIds.length + body.groupIds.length, existing: 0 }),
      );
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
  return server;
}

function renderLibrary(audience: Audience = "owner") {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
                <Route path="/videos/:id" element={<p>再生画面</p>} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function useListView() {
  localStorage.setItem(
    "vv.view.v2",
    JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
  );
}

async function choose(user: ReturnType<typeof userEvent.setup>, name = "Tonight") {
  await user.click(await screen.findByRole("option", { name }));
}

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  vi.stubGlobal("fetch", fetchMock);
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
  fetchMock.mockReset();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("ライブラリからプレイリストに足す", () => {
  it("動画のカードでは、ハートの左の印から選ぶとその動画の id を送り、カードは開かない", async () => {
    const server = install([{ kind: "video", video: video(1) }]);
    const user = userEvent.setup();
    renderLibrary();
    const trigger = await screen.findByRole("button", {
      name: 'Add "動画 1" to a playlist',
    });
    const heart = screen.getByRole("button", { name: 'Favorite "動画 1"' });
    expect(trigger.parentElement).toBe(heart.parentElement);
    expect(trigger.parentElement?.getAttribute("data-corner")).toBe("top-end");
    expect(trigger.nextElementSibling).toBe(heart);
    expect(trigger.className).toContain("opacity-0");
    expect(trigger.className).toContain("[@media(hover:none)]:opacity-100");
    expect(trigger.className).toContain("size-8");
    expect(trigger.closest("a")).toBeNull();

    await user.click(trigger);
    expect(trigger.className).not.toContain("opacity-0");
    await choose(user);
    expect(await screen.findByText('Added 1 item to "Tonight"')).toBeDefined();
    expect(server.adds).toEqual([{ videoIds: [1], groupIds: [] }]);
    expect(screen.queryByText("再生画面")).toBeNull();
  });

  it("グループのカードからは、そのグループの id を送る", async () => {
    const server = install([{ kind: "group", group: seriesGroup() }]);
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("button", { name: 'Add group "series" to a playlist' }),
    );
    await choose(user);
    expect(await screen.findByText('Added 1 item to "Tonight"')).toBeDefined();
    expect(server.adds).toEqual([{ videoIds: [], groupIds: [7] }]);
  });

  it("動画 2 本とグループ 1 つを選ぶと、2 つの動画の id とグループの id を 1 回の要求で送り、選択は残る", async () => {
    const server = install([
      { kind: "video", video: video(1) },
      { kind: "video", video: video(2) },
      { kind: "group", group: seriesGroup() },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await screen.findByRole("checkbox", { name: 'Select "動画 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 2"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "series"' }));

    const bar = screen.getByRole("region", { name: "Selection actions" });
    const names = within(bar)
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));
    expect(names.indexOf("Add to playlist")).toBe(names.indexOf("Favorite") + 1);
    expect(names.indexOf("Visibility")).toBe(names.indexOf("Add to playlist") + 3);

    await user.click(within(bar).getByRole("button", { name: "Add to playlist" }));
    await choose(user);
    expect(await screen.findByText('Added 3 items to "Tonight"')).toBeDefined();
    expect(server.adds).toEqual([{ videoIds: [1, 2], groupIds: [7] }]);
    expect(screen.getByText("5 videos selected")).toBeDefined();
  });

  it("リスト表示では、お気に入りの列の直後の列に置く", async () => {
    useListView();
    const server = install([
      { kind: "video", video: video(1) },
      { kind: "group", group: seriesGroup() },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    const trigger = await screen.findByRole("button", {
      name: 'Add "動画 1" to a playlist',
    });
    const cell = trigger.closest("td") as HTMLElement;
    expect(cell.className).toContain("w-8");
    expect(
      within(cell.previousElementSibling as HTMLElement).getByRole("button", {
        name: 'Favorite "動画 1"',
      }),
    ).toBeDefined();
    expect(trigger.className).toContain("size-6");
    expect(trigger.className).toContain("text-muted-foreground");
    const groupCell = screen
      .getByRole("button", { name: 'Add group "series" to a playlist' })
      .closest("td") as HTMLElement;
    const heads = screen.getAllByRole("columnheader");
    const row = trigger.closest("tr") as HTMLElement;
    expect(row.children).toHaveLength(heads.length);
    // Hide the column below sm so it cannot push duration off a 360px screen.
    //
    const head = heads[Array.from(row.children).indexOf(cell)] as HTMLElement;
    for (const element of [cell, groupCell, head]) {
      expect(element.className.split(" ")).toEqual(
        expect.arrayContaining(["hidden", "sm:table-cell"]),
      );
    }
    expect((cell.previousElementSibling as HTMLElement).className).not.toContain(
      "hidden",
    );

    await user.click(trigger);
    await choose(user);
    await waitFor(() => expect(server.adds).toEqual([{ videoIds: [1], groupIds: [] }]));
  });

  it("ゲストのカードと行には追加の操作が無い", async () => {
    const items: LibraryItem[] = [
      { kind: "video", video: video(1, { public: true, favorite: undefined }) },
    ];
    install(items);
    renderLibrary("guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: /to a playlist$/ })).toBeNull();
    cleanup();

    useListView();
    renderLibrary("guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: /to a playlist$/ })).toBeNull();
    expect(
      fetchMock.mock.calls.some(([input]) => String(input).includes("playlists")),
    ).toBe(false);
  });
});
