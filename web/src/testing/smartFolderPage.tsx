/**
 * smartFolderPage はスマートフォルダのページ（src/smartFolders/SmartFolderPage.tsx）とカードの
 * テストが共有する偽のサーバーと、画面に入ったことにする見張りである。
 */
import { act, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import { installFakeEventSource } from "../api/fakeEventSource";
import type { LibrarySmartFolder, SmartFolderDetail } from "../api/smartFolders";
import { type Audience, AudienceProvider } from "../auth/audience";
import SmartFolderPage from "../smartFolders/SmartFolderPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { json, Probe, video } from "./groupPage";

export { video };

/** folderCard はカードの値である。 */
export function folderCard(
  id: number,
  name: string,
  extra: Partial<LibrarySmartFolder> = {},
): LibrarySmartFolder {
  return {
    id,
    name,
    videoCount: 2,
    watchedCount: 0,
    watchState: "unwatched",
    durationMs: 120_000,
    sizeBytes: 2048,
    addedAt: "2026-09-01T00:00:00Z",
    previews: [],
    openVideoId: 10,
    tags: [],
    ...extra,
  };
}

export const server = {
  /** GET /api/smart-folders/{id} の応答（id ごと）。 */
  folders: new Map<number, SmartFolderDetail>(),
  /** GET /api/smart-folders/cards が答える値（id ごと）。 */
  cards: new Map<number, LibrarySmartFolder>(),
  /** GET /api/smart-folders/cards の `ids`（要求ごと）。 */
  cardRequests: [] as number[][],
  /** GET /api/smart-folders/{id} の回数（id ごと）。 */
  reads: new Map<number, number>(),
  /** GET /api/tags の項目。 */
  tags: [] as { id: number; name: string }[],
};

function folderResponse(url: URL): Response | undefined {
  if (url.pathname === "/api/smart-folders/cards") {
    const ids = (url.searchParams.get("ids") ?? "").split(",").map(Number);
    server.cardRequests.push(ids);
    return json({
      cards: ids.flatMap((id) => {
        const card = server.cards.get(id);
        return card === undefined ? [] : [card];
      }),
    });
  }
  const detail = /^\/api\/smart-folders\/(\d+)$/.exec(url.pathname);
  if (detail === null) return undefined;
  const id = Number(detail[1]);
  server.reads.set(id, (server.reads.get(id) ?? 0) + 1);
  const stored = server.folders.get(id);
  return stored === undefined
    ? json({ code: "not_found", message: "x" }, 404)
    : json(stored);
}

function install() {
  const fetchMock = vi.fn<typeof fetch>((input) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname === "/api/tags") {
      return Promise.resolve(
        json({ items: server.tags.map((tag) => ({ ...tag, videoCount: 1 })) }),
      );
    }
    const answer = folderResponse(url);
    if (answer !== undefined) return Promise.resolve(answer);
    throw new Error(`unexpected request: ${url.toString()}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

/** FakeObserver は見張る要素を覚え、`revealCards` で画面に入ったことにする。 */
class FakeObserver {
  static all = new Set<FakeObserver>();
  private readonly callback: IntersectionObserverCallback;
  readonly elements = new Set<Element>();
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
  }
  observe(element: Element) {
    this.elements.add(element);
    FakeObserver.all.add(this);
  }
  disconnect() {
    this.elements.clear();
    FakeObserver.all.delete(this);
  }
  fire(element: Element) {
    this.callback(
      [{ isIntersecting: true, target: element } as unknown as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}

/**
 * revealCards はスマートフォルダのカード（`ids`、省けばすべて）が画面に入ったことにし、
 * まとめた読み出しが答えるまで待つ。
 */
export async function revealCards(ids?: readonly number[]) {
  await act(async () => {
    for (const observer of [...FakeObserver.all]) {
      for (const element of [...observer.elements]) {
        const id = Number(element.getAttribute("data-smart-folder-id"));
        if (id === 0 || (ids !== undefined && !ids.includes(id))) continue;
        observer.fire(element);
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/** setUpSmartFolderServer は各テストの前に偽のサーバーを初期状態へ戻し、後で差し替えを解く。 */
export function setUpSmartFolderServer() {
  beforeEach(() => {
    server.folders = new Map();
    server.cards = new Map();
    server.cardRequests = [];
    server.reads = new Map();
    server.tags = [];
    FakeObserver.all = new Set();
    vi.stubGlobal("IntersectionObserver", FakeObserver);
    install();
    installFakeEventSource();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
}

export function renderPage(initial: string, audience: Audience = "owner") {
  render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/smart-folders/:id" element={<SmartFolderPage />} />
              <Route path="/videos/:id" element={<Probe />} />
              <Route path="/groups/:id" element={<Probe />} />
              <Route path="/" element={<p>Library</p>} />
            </Routes>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/**
 * seed は「タグ: 猫」（id 1）に子「未視聴」（id 5）と動画 2 本を、「未視聴」に動画 1 本を置く。
 */
export function seed() {
  server.tags = [{ id: 3, name: "猫" }];
  server.folders.set(1, {
    smartFolder: folderCard(1, "タグ: 猫", { videoCount: 2, durationMs: 5_530_000 }),
    settings: { rule: { tags: [3] }, sort: "addedDesc", missing: [] },
    trail: [],
    items: [
      { kind: "smartFolder", smartFolder: { id: 5, parentId: 1, name: "未視聴" } },
      { kind: "video", video: video(10, { title: "Cat 1" }) },
      { kind: "video", video: video(11, { title: "Cat 2" }) },
    ],
  });
  server.folders.set(5, {
    smartFolder: folderCard(5, "未視聴", { videoCount: 1, openVideoId: 10 }),
    settings: { rule: { watch: "unwatched" }, sort: "addedDesc", missing: [] },
    trail: [{ id: 1, name: "タグ: 猫" }],
    items: [{ kind: "video", video: video(10, { title: "Cat 1" }) }],
  });
  server.cards.set(5, folderCard(5, "未視聴", { videoCount: 1 }));
}

export function trailBand() {
  return screen.getByRole("navigation", { name: "Smart folder path" });
}

export function crumbs(): string[] {
  return within(trailBand())
    .getAllByRole("listitem")
    .map((item) => item.textContent ?? "")
    .filter((text) => text !== "");
}
