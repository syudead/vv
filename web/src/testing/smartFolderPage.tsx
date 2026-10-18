import { act, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import { installFakeEventSource } from "../api/fakeEventSource";
import type {
  LibrarySmartFolder,
  SmartFolderDetail,
  SmartFolderItemPage,
  SmartFolderSummary,
} from "../api/smartFolders";
import { type Audience, AudienceProvider } from "../auth/audience";
import SmartFolderPage from "../smartFolders/SmartFolderPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { json, Probe, video } from "./groupPage";

export { video };

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
  folders: new Map<number, SmartFolderDetail>(),
  cards: new Map<number, LibrarySmartFolder>(),
  cardRequests: [] as number[][],
  reads: new Map<number, number>(),
  tags: [] as { id: number; name: string }[],
  pages: new Map<string, SmartFolderItemPage>(),
  writes: [] as { method: string; path: string; body: unknown }[],
  /** Capture card values at request time and hold delivery so tests control response order. */
  holdCards: false,
  heldCards: [] as (() => void)[],
  failCards: false,
  failDetail: false,
  failPage: false,
  homeShelves: [] as number[],
  writeAnswer: undefined as ((method: string, id: number) => Response) | undefined,
};

function summaries(): SmartFolderSummary[] {
  return [...server.folders.values()].map((detail) => ({
    id: detail.smartFolder.id,
    name: detail.smartFolder.name,
    parentId: detail.trail.at(-1)?.id,
  }));
}

function writeResponse(method: string, url: URL, body: unknown): Response {
  if (url.pathname === "/api/home/shelves") {
    server.writes.push({ method, path: url.pathname, body });
    server.homeShelves = (body as { folderIds: number[] }).folderIds;
    for (const [id, detail] of server.folders) {
      detail.onHome = server.homeShelves.includes(id);
    }
    return new Response(null, { status: 204 });
  }
  const match = /^\/api\/smart-folders\/(\d+)$/.exec(url.pathname);
  const id = match === null ? 0 : Number(match[1]);
  server.writes.push({ method, path: url.pathname, body });
  if (server.writeAnswer !== undefined) return server.writeAnswer(method, id);
  if (method === "POST") {
    const created = body as { name: string; parentId?: number };
    return json({ id: 90, name: created.name, parentId: created.parentId }, 201);
  }
  const stored = server.folders.get(id);
  if (stored === undefined) return json({ code: "not_found", message: "x" }, 404);
  if (method === "DELETE") {
    server.folders.delete(id);
    return new Response(null, { status: 204 });
  }
  const change = body as { name?: string };
  if (change.name !== undefined) {
    stored.smartFolder = { ...stored.smartFolder, name: change.name };
  }
  return json({ id, name: stored.smartFolder.name, parentId: stored.trail.at(-1)?.id });
}

function folderResponse(url: URL): Response | undefined {
  if (url.pathname === "/api/smart-folders/cards") {
    const ids = (url.searchParams.get("ids") ?? "").split(",").map(Number);
    server.cardRequests.push(ids);
    if (server.failCards) return json({ code: "internal", message: "x" }, 500);
    return json({
      cards: ids.flatMap((id) => {
        const card = server.cards.get(id);
        return card === undefined ? [] : [card];
      }),
    });
  }
  if (url.pathname === "/api/smart-folders") return json({ smartFolders: summaries() });
  if (url.pathname === "/api/home") {
    return json({
      shelves: server.homeShelves.map((id) => ({
        key: `folder:${String(id)}`,

        name: server.folders.get(id)?.smartFolder.name ?? "",
        folderId: id,
      })),
    });
  }
  const items = /^\/api\/smart-folders\/(\d+)\/items$/.exec(url.pathname);
  if (items !== null) {
    if (server.failPage) return json({ code: "internal", message: "x" }, 500);
    const page = server.pages.get(
      `${items[1] ?? ""}:${url.searchParams.get("cursor") ?? ""}`,
    );
    return page === undefined
      ? json({ code: "not_found", message: "x" }, 404)
      : json(page);
  }
  const detail = /^\/api\/smart-folders\/(\d+)$/.exec(url.pathname);
  if (detail === null) return undefined;
  const id = Number(detail[1]);
  server.reads.set(id, (server.reads.get(id) ?? 0) + 1);
  if (server.failDetail) return json({ code: "internal", message: "x" }, 500);
  const stored = server.folders.get(id);
  return stored === undefined
    ? json({ code: "not_found", message: "x" }, 404)
    : json(stored);
}

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (method !== "GET") {
      const body = init?.body === undefined ? undefined : JSON.parse(String(init.body));
      return Promise.resolve(writeResponse(method, url, body));
    }
    if (url.pathname === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/tags") {
      return Promise.resolve(
        json({
          items: server.tags.map((tag) => ({ ...tag, synonyms: [], videoCount: 1 })),
        }),
      );
    }
    const answer = folderResponse(url);
    if (answer !== undefined && server.holdCards && url.pathname.endsWith("/cards")) {
      return new Promise<Response>((resolve) => {
        server.heldCards.push(() => resolve(answer));
      });
    }
    if (answer !== undefined) return Promise.resolve(answer);
    throw new Error(`unexpected request: ${url.toString()}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

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

/** Reveal requested cards, or all when omitted, and await the batched read. */
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

/** Reveal the pagination sentinel and await its page response. */
export async function revealSentinel() {
  await act(async () => {
    for (const observer of [...FakeObserver.all]) {
      for (const element of [...observer.elements]) {
        if (element.hasAttribute("data-smart-folder-id")) continue;
        observer.fire(element);
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

export function setUpSmartFolderServer() {
  beforeEach(() => {
    server.folders = new Map();
    server.cards = new Map();
    server.cardRequests = [];
    server.reads = new Map();
    server.tags = [];
    server.pages = new Map();
    server.writes = [];
    server.homeShelves = [];
    server.writeAnswer = undefined;
    server.failDetail = false;
    server.failPage = false;
    server.holdCards = false;
    server.heldCards = [];
    server.failCards = false;
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
