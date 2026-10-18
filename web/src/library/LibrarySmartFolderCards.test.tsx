import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryItem } from "../api/client";
import { clearListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { folderCard, video } from "../testing/smartFolderPage";
import { group, Probe } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

const fetchMock = vi.fn<typeof fetch>();
let cardRequests: string[] = [];

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function install(items: LibraryItem[]) {
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), "http://localhost");
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
    if (url.pathname === "/api/smart-folders/cards") {
      cardRequests.push(url.searchParams.get("ids") ?? "");
      return Promise.resolve(
        json({
          cards: [
            folderCard(9, "猫", {
              videoCount: 4,
              previews: [
                { videoId: 10, thumbnailUrl: "/thumb10", title: "Harbour" },
                { videoId: 11, thumbnailUrl: "/thumb11", title: "Ridge" },
              ],
              tags: [
                {
                  id: 3,
                  name: "旅行",
                  manual: true,
                  fromFolder: false,
                  tentative: false,
                  kind: null,
                },
              ],
            }),
          ],
        }),
      );
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

class RevealingObserver {
  private readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
  }
  observe(element: Element) {
    if (!element.hasAttribute("data-smart-folder-id")) return;
    queueMicrotask(() =>
      this.callback(
        [
          {
            isIntersecting: true,
            target: element,
          } as unknown as IntersectionObserverEntry,
        ],
        this as unknown as IntersectionObserver,
      ),
    );
  }
  disconnect() {}
}

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.search}</span>;
}

function renderLibrary() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
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
                <Route path="/videos/:id" element={<Probe />} />
                <Route path="/smart-folders/:id" element={<p>Smart folder page</p>} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const items: LibraryItem[] = [
  { kind: "video", video: video(1, { title: "Episode" }) },
  { kind: "smartFolder", smartFolder: { id: 9, name: "猫" } },
  { kind: "group", group: group(2, "Season") },
];

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  cardRequests = [];
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("IntersectionObserver", RevealingObserver);
});

afterEach(() => {
  cleanup();
  fetchMock.mockReset();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("ライブラリのスマートフォルダのカード", () => {
  it("グループや動画の間に、一覧の順でカードを出し、値を読んで本数を出す", async () => {
    install(items);
    renderLibrary();

    const folder = await screen.findByRole("link", {
      name: "猫, smart folder of 4 videos",
    });
    const names = screen
      .getAllByRole("article")
      .map((card) => within(card).getAllByRole("link")[0]?.getAttribute("aria-label"));
    expect(names).toEqual([
      expect.stringContaining("Episode"),
      "猫, smart folder of 4 videos",
      expect.stringContaining("Season"),
    ]);
    expect(cardRequests).toEqual(["9"]);
    const card = folder.closest("article") as HTMLElement;
    expect(within(card).queryByRole("checkbox")).toBeNull();
  });

  it("shows one inline folder and opens a preview with autoplay and folder playback context", async () => {
    install(items);
    renderLibrary();
    const folder = await screen.findByRole("link", {
      name: "猫, smart folder of 4 videos",
    });
    const art = folder.querySelector<HTMLElement>("[data-folder-art]")!;
    art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
    expect(screen.queryByRole("region", { name: "Shelves" })).toBeNull();
    expect(
      screen.getAllByRole("link", { name: "猫, smart folder of 4 videos" }),
    ).toHaveLength(1);
    expect(folder.closest("article")?.querySelectorAll("a")).toHaveLength(1);
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: 10 });
    const preview = screen.getByRole("link", { name: 'Play "Harbour" from "猫"' });
    fireEvent.click(preview);
    expect(
      await screen.findByText(
        'Opened /videos/10?smartFolder=9 {"from":"/","autoplay":true}',
      ),
    ).toBeTruthy();
  });

  it("カードはスマートフォルダのページを開く", async () => {
    const user = userEvent.setup();
    install(items);
    renderLibrary();

    await user.click(await screen.findByRole("link", { name: /^猫, smart folder/ }));

    expect(await screen.findByText("Smart folder page")).toBeTruthy();
  });

  it("リスト表示ではグループの行と同じ列の行を出す", async () => {
    localStorage.setItem(
      "vv.view.v2",
      JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
    );
    install(items);
    renderLibrary();

    const link = await screen.findByRole("link", {
      name: "猫, smart folder of 4 videos",
    });
    const row = link.closest("tr") as HTMLElement;
    await waitFor(() => expect(within(row).getByText("4 videos")).toBeTruthy());
    expect(within(row).queryByRole("checkbox")).toBeNull();
  });

  it("ほかのカードを選んでいる間も、フォルダのカードのタグを押すとそのタグで絞り込む", async () => {
    const user = userEvent.setup();
    install(items);
    renderLibrary();
    const folder = await screen.findByRole("link", {
      name: "猫, smart folder of 4 videos",
    });
    const card = folder.closest("article") as HTMLElement;

    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode"' }));
    await user.click(within(card).getByRole("button", { name: "Filter by 旅行" }));

    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toContain("tag=3"),
    );
  });
});
