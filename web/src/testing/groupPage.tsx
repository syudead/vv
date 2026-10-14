/**
 * groupPage はグループのページ（src/groups/GroupPage.tsx）のテストが共有する偽のサーバーと
 * 操作の手順である。テストは話題ごとのファイル（src/groups/GroupPage*.test.tsx）に分ける。
 */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import type { GroupItem } from "../api/groups";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import GroupPage from "../groups/GroupPage";

export function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `Video ${String(id)}`,
    public: true,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    container: "mp4",
    videoCodec: "h264",
    tags: [],
    ...extra,
  };
}

export function group(
  id: number,
  name: string,
  extra: Partial<LibraryGroup> = {},
): LibraryGroup {
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
    videoIds: [10, 11],
    tags: [],
    ...extra,
  };
}

/** StoredImage は取り消しで戻すグループ 1 つの、操作の前の値である。 */
interface StoredImage {
  groupId: number;
  stored: StoredGroup;
}

export interface StoredGroup {
  group: LibraryGroup;
  items: GroupItem[];
  /** 最初のページの続き。置くと GET /api/groups/{id}/items?cursor= がそのページを返す。 */
  nextCursor?: string;
}

export const server = {
  groups: new Map<number, StoredGroup>(),
  /** GET /api/groups/{id} の回数（id ごと）。 */
  reads: new Map<number, number>(),
  /** 続きのページ（cursor ごと）。 */
  pages: new Map<string, { items: GroupItem[]; nextCursor?: string }>(),
  /** 続きのページの要求の cursor。 */
  pageRequests: [] as string[],
  /** true の間、続きのページは 500 で失敗する。 */
  pagesFail: false,
  /** 書き込みの要求（メソッド、パス、本文）。 */
  writes: [] as { method: string; path: string; body?: unknown }[],
  /** 取り消しのトークンごとの、操作の前のグループ（解散したグループも戻す）。 */
  undo: new Map<string, StoredImage[]>(),
  /** 置くと POST /api/organize/undo がこの応答を返し、何も戻さない。 */
  undoAnswer: null as (() => Response) | null,
};

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function isChild(parent: number, child: number): boolean {
  return (
    server.groups
      .get(parent)
      ?.items.some((item) => item.kind === "group" && item.group?.id === child) ?? false
  );
}

/** trailOf は契約どおり、このグループから外へ向かって親と子の組が続く部分を返す。 */
function trailOf(id: number, requested: number[]) {
  const chain = [...requested, id];
  const kept: number[] = [];
  for (let index = chain.length - 1; index > 0; index -= 1) {
    const parent = chain[index - 1] as number;
    if (!isChild(parent, chain[index] as number)) break;
    kept.unshift(parent);
  }
  return kept.map((each) => ({
    id: each,
    name: server.groups.get(each)?.group.name ?? "",
  }));
}

/** summarize は項目からグループの動画の id と本数を数え直した値を返す。 */
export function summarize(stored: StoredGroup): LibraryGroup {
  const videoIds = stored.items.flatMap((item) =>
    item.kind === "group" ? (item.group?.videoIds ?? []) : [item.video?.id ?? 0],
  );
  stored.group = { ...stored.group, videoIds, videoCount: videoIds.length };
  return stored.group;
}

// サーバーは code を not_found にし、どの 404 かを reason に入れる（writeGroupError）。
export const targetGone = () =>
  Promise.resolve(
    json({ code: "not_found", reason: "group_target_not_found", message: "x" }, 404),
  );
export const groupNotFound = () =>
  Promise.resolve(json({ code: "not_found", message: "x" }, 404));

/** keepImage は操作の前のグループ `ids` を、新しいトークンで覚えてそのトークンを返す。 */
function keepImage(ids: readonly number[]): string {
  const token = `token-${String(server.undo.size + 1)}`;
  server.undo.set(
    token,
    ids.flatMap((groupId) => {
      const stored = server.groups.get(groupId);
      return stored === undefined ? [] : [{ groupId, stored: { ...stored } }];
    }),
  );
  return token;
}

interface MoveBody {
  items: { kind: "video" | "group"; id: number }[];
  from?: number;
  to?: number;
}

/** refMatches は項目が操作の指す動画かグループかを返す。 */
function refMatches(item: GroupItem, ref: MoveBody["items"][number]): boolean {
  return ref.kind === "video"
    ? item.kind === "video" && item.video?.id === ref.id
    : item.kind === "group" && item.group?.id === ref.id;
}

/**
 * moveRefs は POST /api/organize/move に答える。`from` から外し、`to` の末尾へ足す（すでに
 * ある動画は足さない）。`from` が空になったら解散する。
 */
function moveRefs({ items, from, to }: MoveBody): Promise<Response> {
  const source = from === undefined ? undefined : server.groups.get(from);
  const target = to === undefined ? undefined : server.groups.get(to);
  if ((from !== undefined && source === undefined) || (to !== undefined && !target)) {
    return targetGone();
  }
  if (items.some((ref) => ref.kind === "group" && ref.id === to)) {
    return Promise.resolve(
      json({ code: "conflict", reason: "group_cycle", message: "x" }, 409),
    );
  }
  const token = keepImage([from, to].filter((each) => each !== undefined));
  let nextId = 900 + server.writes.length * 10;
  if (source !== undefined && from !== undefined) {
    source.items = source.items.filter(
      (item) => !items.some((ref) => refMatches(item, ref)),
    );
    if (source.items.length === 0) server.groups.delete(from);
  }
  if (target !== undefined) {
    for (const ref of items) {
      if (target.items.some((item) => refMatches(item, ref))) continue;
      nextId += 1;
      target.items = [
        ...target.items,
        ref.kind === "video"
          ? { id: nextId, kind: "video", video: video(ref.id) }
          : { id: nextId, kind: "group", group: server.groups.get(ref.id)?.group },
      ];
    }
    summarize(target);
  }
  return Promise.resolve(json({ undoToken: token }));
}

/**
 * mergeRefs は POST /api/organize/merge に答える。グループは項目を `into` の末尾へ渡し、親の
 * 項目から消えてなくなる。動画は `into` の末尾へ足す（どちらもすでにある動画は足さない）。
 */
function mergeRefs({ items, into }: { items: MoveBody["items"]; into: number }) {
  const target = server.groups.get(into);
  if (
    target === undefined ||
    items.some((ref) => ref.kind === "group" && ref.id === into)
  ) {
    return targetGone();
  }
  const merged = items.filter((ref) => ref.kind === "group").map((ref) => ref.id);
  const token = keepImage(Array.from(server.groups.keys()));
  let nextId = 900 + server.writes.length * 10;
  const append = (item: GroupItem) => {
    if (
      target.items.some(
        (each) => each.video !== undefined && each.video.id === item.video?.id,
      )
    ) {
      return;
    }
    nextId += 1;
    target.items = [...target.items, { ...item, id: nextId }];
  };
  for (const ref of items) {
    if (ref.kind === "video") append({ id: 0, kind: "video", video: video(ref.id) });
    else server.groups.get(ref.id)?.items.forEach(append);
  }
  for (const id of merged) server.groups.delete(id);
  for (const stored of server.groups.values()) {
    stored.items = stored.items.filter(
      (item) => item.group === undefined || !merged.includes(item.group.id),
    );
  }
  summarize(target);
  return Promise.resolve(json({ undoToken: token }));
}

/** organize は POST /api/organize/reorder・/move・/merge・/undo に、メモリ上のグループを変えて答える。 */
function organize(path: string, body: unknown): Promise<Response> | undefined {
  if (path === "/api/organize/move") return moveRefs(body as MoveBody);
  if (path === "/api/organize/merge") {
    return mergeRefs(body as { items: MoveBody["items"]; into: number });
  }
  if (path === "/api/organize/reorder") {
    const { groupId, itemIds, after } = body as {
      groupId: number;
      itemIds: number[];
      after?: number;
    };
    const stored = server.groups.get(groupId);
    if (
      stored === undefined ||
      !itemIds.every((each) => stored.items.some((item) => item.id === each))
    ) {
      return targetGone();
    }
    const token = keepImage([groupId]);
    const moving = itemIds.map(
      (each) => stored.items.find((item) => item.id === each) as GroupItem,
    );
    const others = stored.items.filter((item) => !itemIds.includes(item.id));
    const index =
      after === undefined ? 0 : others.findIndex((item) => item.id === after) + 1;
    stored.items = [...others.slice(0, index), ...moving, ...others.slice(index)];
    return Promise.resolve(json({ undoToken: token }));
  }
  if (path === "/api/organize/undo") {
    if (server.undoAnswer !== null) return Promise.resolve(server.undoAnswer());
    const images = server.undo.get((body as { token: string }).token);
    if (images === undefined) {
      return Promise.resolve(
        json({ code: "not_found", reason: "undo_expired", message: "x" }, 410),
      );
    }
    for (const { groupId, stored } of images) server.groups.set(groupId, stored);
    return Promise.resolve(new Response(null, { status: 204 }));
  }
  return undefined;
}

function write(url: URL, method: string, body: unknown): Promise<Response> | undefined {
  if (url.pathname.startsWith("/api/organize/")) {
    server.writes.push({ method, path: url.pathname, body });
    return organize(url.pathname, body);
  }
  const match = /^\/api\/groups\/(\d+)(\/.*)?$/.exec(url.pathname);
  if (match === null) return undefined;
  server.writes.push({ method, path: url.pathname, body });
  const id = Number(match[1]);
  const rest = match[2] ?? "";
  const stored = server.groups.get(id);
  if (stored === undefined) return groupNotFound();
  if (method === "PATCH" && rest === "") {
    stored.group = { ...stored.group, name: (body as { name: string }).name };
    return Promise.resolve(json(stored.group));
  }
  if (method === "DELETE" && rest === "") {
    server.groups.delete(id);
    return Promise.resolve(new Response(null, { status: 204 }));
  }
  if (rest === "/tag") {
    server.groups.delete(id);
    return Promise.resolve(
      json({ tag: { id: 3, name: stored.group.name }, created: true }),
    );
  }
  if (rest === "/items/remove") {
    const itemIds = (body as { itemIds: number[] }).itemIds;
    if (!itemIds.every((each) => stored.items.some((item) => item.id === each))) {
      return targetGone();
    }
    stored.items = stored.items.filter((item) => !itemIds.includes(item.id));
    // 最後の項目を外したグループは解散され、本文の無い 204 で答える（data-model.md「Edits」）。
    if (stored.items.length === 0) {
      server.groups.delete(id);
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    return Promise.resolve(json(summarize(stored)));
  }
  const move = /^\/items\/(\d+)\/move$/.exec(rest);
  if (move !== null) {
    const itemId = Number(move[1]);
    const after = (body as { after?: number }).after ?? null;
    const moving = stored.items.find((item) => item.id === itemId);
    if (moving === undefined) return targetGone();
    const others = stored.items.filter((item) => item.id !== itemId);
    const index = after === null ? 0 : others.findIndex((item) => item.id === after) + 1;
    stored.items = [...others.slice(0, index), moving, ...others.slice(index)];
    return Promise.resolve(new Response(null, { status: 204 }));
  }
  return undefined;
}

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (method !== "GET") {
      const body =
        typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined;
      const answer = write(url, method, body);
      if (answer !== undefined) return answer;
    }
    if (url.pathname === "/api/tags") return Promise.resolve(json([]));
    const detail = /^\/api\/groups\/(\d+)$/.exec(url.pathname);
    if (detail !== null) {
      const id = Number(detail[1]);
      server.reads.set(id, (server.reads.get(id) ?? 0) + 1);
      const stored = server.groups.get(id);
      if (stored === undefined) {
        return Promise.resolve(json({ code: "not_found", message: "x" }, 404));
      }
      const requested = (url.searchParams.get("trail") ?? "")
        .split(",")
        .filter((each) => each !== "")
        .map(Number);
      return Promise.resolve(
        json({
          group: stored.group,
          trail: trailOf(id, requested),
          items: stored.items,
          nextCursor: stored.nextCursor,
        }),
      );
    }
    if (/^\/api\/groups\/\d+\/items$/.test(url.pathname)) {
      const cursor = url.searchParams.get("cursor") ?? "";
      server.pageRequests.push(cursor);
      const page = server.pages.get(cursor);
      if (server.pagesFail || page === undefined) {
        return Promise.resolve(json({ code: "internal", message: "x" }, 500));
      }
      return Promise.resolve(json(page));
    }
    if (url.pathname === "/api/folders/1") {
      return Promise.resolve(
        json({
          folder: { rootId: 1, path: "", name: "Media", videoCount: 0, folderCount: 0 },
          folders: [],
        }),
      );
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

/** Probe は開いた場所と state を書き出す（再生画面とフォルダ画面の代わり）。 */
export function Probe() {
  const location = useLocation();
  return (
    <p>{`Opened ${location.pathname}${location.search} ${JSON.stringify(location.state)}`}</p>
  );
}

export function renderPage(initial: string, audience: Audience = "owner") {
  render(
    <MemoryRouter initialEntries={[initial]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/groups/:id" element={<GroupPage />} />
              <Route path="/videos/:id" element={<Probe />} />
              <Route path="/folders/*" element={<Probe />} />
              <Route path="/" element={<p>Library</p>} />
            </Routes>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/** seed は「Season」（id 1）の中に子グループ「Extras」（id 5）と動画 2 本を置く。 */
export function seed() {
  const extras = group(5, "Extras", {
    videoCount: 1,
    videoIds: [12],
    openVideoId: 12,
    durationMs: 60_000,
  });
  server.groups.set(1, {
    group: group(1, "Season", {
      videoCount: 3,
      videoIds: [10, 12, 11],
      durationMs: 5_530_000,
    }),
    items: [
      { id: 101, kind: "video", video: video(10, { title: "Episode 1" }) },
      { id: 102, kind: "group", group: extras },
      { id: 103, kind: "video", video: video(11, { title: "Episode 2" }) },
    ],
  });
  server.groups.set(5, {
    group: extras,
    items: [{ id: 201, kind: "video", video: video(12, { title: "Bonus" }) }],
  });
}

export function trailBand() {
  return screen.getByRole("navigation", { name: "Group path" });
}

export function crumbs(): string[] {
  return within(trailBand())
    .getAllByRole("listitem")
    .map((item) => item.textContent ?? "")
    .filter((text) => text !== "");
}

/** 番兵の見張り。`reveal` で見える範囲に来たことにする。 */
let revealNow: () => void = () => undefined;

/** reveal は番兵が見える範囲に来たことにする。 */
export function reveal() {
  revealNow();
}

class FakeIntersectionObserver {
  private readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
  }
  observe() {
    revealNow = () =>
      this.callback(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      );
  }
  disconnect() {
    revealNow = () => undefined;
  }
}

/**
 * setUpGroupPageServer は各テストの前に偽のサーバーを初期状態へ戻し、後で差し替えを解く。
 * テストのファイルの先頭で 1 回呼ぶ。
 */
export function setUpGroupPageServer() {
  beforeEach(() => {
    server.groups = new Map();
    server.reads = new Map();
    server.pages = new Map();
    server.pageRequests = [];
    server.pagesFail = false;
    server.writes = [];
    server.undo = new Map();
    server.undoAnswer = null;
    vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
    install();
    installFakeEventSource();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
}

// jsdom は配置を持たないので、行の位置を上から 48px ずつに置く。dnd-kit のキーボードの
// 移動は行の位置から次の行を探す。
export function layoutRows() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const row = this.closest("tr[data-item-id]");
    const index =
      row === null ? 0 : Array.from(row.parentElement?.children ?? []).indexOf(row);
    const top = index * 48;
    return {
      x: 0,
      y: top,
      top,
      left: 0,
      right: 800,
      bottom: top + 48,
      width: 800,
      height: 48,
      toJSON: () => ({}),
    } as DOMRect;
  });
}

/** listTitles はリスト表示の行の題名を上から返す。 */
export function listTitles(): string[] {
  const table = screen.getByRole("table", { name: "Items" });
  return within(table)
    .getAllByRole("link")
    .map((link) => link.getAttribute("title") ?? "");
}

export type User = ReturnType<typeof userEvent.setup>;

/** openList はページを開いてリスト表示に切り替える。 */
export async function openList(user: User, initial = "/groups/1") {
  renderPage(initial);
  await screen.findByRole("heading", { level: 1 });
  await user.click(await screen.findByRole("radio", { name: "List" }));
  return screen.getByRole("table", { name: "Items" });
}

/** openMenu は見出しの「More」から項目を選ぶ。 */
export async function openMenu(user: User, item: string) {
  await user.click(await screen.findByRole("button", { name: "More" }));
  await user.click(await screen.findByRole("menuitem", { name: item }));
}

/** moveFirstDown はキーボードで「Episode 1」を 1 つ下へ動かす。 */
export async function moveFirstDown(user: User) {
  screen.getByRole("button", { name: 'Reorder "Episode 1"' }).focus();
  await user.keyboard("[Space]");
  await user.keyboard("[ArrowDown]");
  await user.keyboard("[Space]");
}
