import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { GroupItem } from "../api/groups";
import { type Audience, AudienceProvider } from "../auth/audience";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import GroupPage from "./GroupPage";

function video(id: number, extra: Partial<Video> = {}): Video {
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

function group(
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

interface StoredGroup {
  group: LibraryGroup;
  items: GroupItem[];
  /** 最初のページの続き。置くと GET /api/groups/{id}/items?cursor= がそのページを返す。 */
  nextCursor?: string;
}

const server = {
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
};

function json(body: unknown, status = 200): Response {
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
function summarize(stored: StoredGroup): LibraryGroup {
  const videoIds = stored.items.flatMap((item) =>
    item.kind === "group" ? (item.group?.videoIds ?? []) : [item.video?.id ?? 0],
  );
  stored.group = { ...stored.group, videoIds, videoCount: videoIds.length };
  return stored.group;
}

const targetGone = () =>
  Promise.resolve(json({ code: "group_target_not_found", message: "x" }, 404));
const groupNotFound = () =>
  Promise.resolve(json({ code: "not_found", message: "x" }, 404));

/** write は書き込みの要求に、メモリ上のグループを変えて契約どおりに答える。 */
function write(url: URL, method: string, body: unknown): Promise<Response> | undefined {
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
function Probe() {
  const location = useLocation();
  return (
    <p>{`Opened ${location.pathname}${location.search} ${JSON.stringify(location.state)}`}</p>
  );
}

function renderPage(initial: string, audience: Audience = "owner") {
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
function seed() {
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

function trailBand() {
  return screen.getByRole("navigation", { name: "Group path" });
}

function crumbs(): string[] {
  return within(trailBand())
    .getAllByRole("listitem")
    .map((item) => item.textContent ?? "")
    .filter((text) => text !== "");
}

/** 番兵の見張り。`reveal` で見える範囲に来たことにする。 */
let reveal: () => void = () => undefined;

class FakeIntersectionObserver {
  private readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
  }
  observe() {
    reveal = () =>
      this.callback(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      );
  }
  disconnect() {
    reveal = () => undefined;
  }
}

beforeEach(() => {
  server.groups = new Map();
  server.reads = new Map();
  server.pages = new Map();
  server.pageRequests = [];
  server.pagesFail = false;
  server.writes = [];
  vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
  install();
  installFakeEventSource();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// jsdom は配置を持たないので、行の位置を上から 48px ずつに置く。dnd-kit のキーボードの
// 移動は行の位置から次の行を探す。
function layoutRows() {
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
function listTitles(): string[] {
  const table = screen.getByRole("table", { name: "Items" });
  return within(table)
    .getAllByRole("link")
    .map((link) => link.getAttribute("title") ?? "");
}

type User = ReturnType<typeof userEvent.setup>;

/** openList はページを開いてリスト表示に切り替える。 */
async function openList(user: User, initial = "/groups/1") {
  renderPage(initial);
  await screen.findByRole("heading", { level: 1 });
  await user.click(await screen.findByRole("radio", { name: "List" }));
  return screen.getByRole("table", { name: "Items" });
}

/** openMenu は見出しの「More」から項目を選ぶ。 */
async function openMenu(user: User, item: string) {
  await user.click(await screen.findByRole("button", { name: "More" }));
  await user.click(await screen.findByRole("menuitem", { name: item }));
}

/** moveFirstDown はキーボードで「Episode 1」を 1 つ下へ動かす。 */
async function moveFirstDown(user: User) {
  screen.getByRole("button", { name: 'Reorder "Episode 1"' }).focus();
  await user.keyboard("[Space]");
  await user.keyboard("[ArrowDown]");
  await user.keyboard("[Space]");
}

describe("GroupPage", () => {
  it("子グループと動画を応答の順に並べ、見出しに名前と本数と長さを出す", async () => {
    seed();
    renderPage("/groups/1");
    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(screen.getByText("3 videos · 1:32:10")).toBeDefined();
    const cards = Array.from(document.querySelectorAll("article"));
    expect(cards.map((card) => card.querySelector("h3")?.textContent)).toEqual([
      "Episode 1",
      "Extras",
      "Episode 2",
    ]);
    expect(crumbs()).toEqual(["Library", "Season"]);
    expect(document.title).toBe("Season");
  });

  it("子グループを開くと帯が 1 段伸び、帯のリンクで上へ戻れる", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await user.click(
      await screen.findByRole("link", { name: "Extras, group of 1 video" }),
    );
    expect(
      await screen.findByRole("heading", { level: 1, name: "Extras" }),
    ).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season", "Extras"]);
    expect(screen.getByRole("link", { name: "Bonus" }).getAttribute("href")).toBe(
      "/videos/12?group=5",
    );

    await user.click(within(trailBand()).getByRole("link", { name: "Season" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season"]);
  });

  it("もう親でないグループの段は、何も言わずに帯から外す", async () => {
    seed();
    server.groups.set(9, { group: group(9, "Old parent"), items: [] });
    renderPage("/groups/5?trail=9,1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    expect(crumbs()).toEqual(["Library", "Season", "Extras"]);
  });

  it("ページから開いた動画と「Play」は ?group を運び、戻る先はこのページ", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    const play = await screen.findByRole("link", { name: 'Play "Season"' });
    expect(play.getAttribute("href")).toBe("/videos/10?group=1");

    await user.click(screen.getByRole("link", { name: "Episode 2" }));
    expect(
      await screen.findByText('Opened /videos/11?group=1 {"from":"/groups/1"}'),
    ).toBeDefined();
  });

  it("表示中の id を運ぶ group の知らせで、選択を残したまま読み直す", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 1"' }));
    expect(server.reads.get(1)).toBe(1);

    // 別のグループの知らせでは読まない。
    await emitServerEvent("group", { ids: [99] });
    expect(server.reads.get(1)).toBe(1);

    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, name: "Season (renamed)" };
    stored.items = [...stored.items].reverse();
    await emitServerEvent("group", { ids: [1, 99] });

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season (renamed)" }),
    ).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season (renamed)"]);
    const titles = Array.from(document.querySelectorAll("article h3")).map(
      (heading) => heading.textContent,
    );
    expect(titles).toEqual(["Episode 2", "Extras", "Episode 1"]);
    expect(
      (
        screen.getByRole("checkbox", { name: 'Select "Episode 1"' }) as HTMLButtonElement
      ).getAttribute("aria-checked"),
    ).toBe("true");
  });

  it("読み直しで外れた動画は選択から外れ、増えた動画は「Select all」でまた選べる", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 1"' }));
    const selectAll = screen.getByRole("button", { name: "Select all" });
    await user.click(selectAll);
    expect(screen.getByText("3 videos selected")).toBeDefined();
    expect(selectAll).toHaveProperty("disabled", true);

    // 別のタブで Episode 1 が外れ、Episode 3 が加わった。
    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, videoCount: 3, videoIds: [12, 11, 13] };
    stored.items = [
      stored.items[1] as GroupItem,
      stored.items[2] as GroupItem,
      { id: 104, kind: "video", video: video(13, { title: "Episode 3" }) },
    ];
    await emitServerEvent("group", { ids: [1] });

    await screen.findByRole("checkbox", { name: 'Select "Episode 3"' });
    expect(await screen.findByText("2 videos selected")).toBeDefined();
    expect(screen.getByRole("button", { name: "Select all" })).toHaveProperty(
      "disabled",
      false,
    );
    await user.click(screen.getByRole("button", { name: "Select all" }));
    expect(screen.getByText("3 videos selected")).toBeDefined();
  });

  it("404 ならグループがもう無いと示し、名前を出さない", async () => {
    renderPage("/groups/404?trail=1");
    expect(await screen.findByText("This group no longer exists")).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Back to library" }).getAttribute("href"),
    ).toBe("/");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Library");
    expect(crumbs()).toEqual(["Library"]);
    expect(screen.queryByRole("link", { name: /^Play/ })).toBeNull();
  });

  it("ゲストは編集の操作なしでページを見る", async () => {
    seed();
    renderPage("/groups/1", "guest");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    expect(screen.getByRole("link", { name: 'Play "Season"' })).toBeDefined();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
    expect(screen.queryByRole("button", { name: /^Favorite/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /to a playlist$/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();

    // リスト表示も番号つきの行だけで、チェックを描かない。
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "List" }));
    const table = screen.getByRole("table", { name: "Items" });
    expect(within(table).getAllByRole("row")).toHaveLength(3);
    expect(within(table).queryAllByRole("checkbox")).toHaveLength(0);
    expect(within(table).getByText("1")).toBeDefined();
  });

  it("自動グループはフォルダの行を出し、手で作ったグループは出さない", async () => {
    seed();
    const user = userEvent.setup();
    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, folder: { rootId: 1, path: "anime/season" } };
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    expect(screen.getByText("From the folder")).toBeDefined();
    const folderLink = await screen.findByRole("link", { name: "Media/anime/season" });
    expect(folderLink.getAttribute("href")).toBe("/folders/1/anime/season");

    await user.click(screen.getByRole("link", { name: "Extras, group of 1 video" }));
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    expect(screen.queryByText("From the folder")).toBeNull();
  });

  it("番兵が近づくと nextCursor で続きを読み、失敗は Retry で読み直す", async () => {
    seed();
    const stored = server.groups.get(1) as StoredGroup;
    stored.nextCursor = "c1";
    server.pages.set("c1", {
      items: [{ id: 104, kind: "video", video: video(13, { title: "Episode 3" }) }],
    });
    server.pagesFail = true;
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });

    act(() => reveal());
    expect(await screen.findByText("Couldn't load more of the group")).toBeDefined();
    server.pagesFail = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("link", { name: "Episode 3" })).toBeDefined();
    expect(server.pageRequests).toEqual(["c1", "c1"]);
    expect(document.querySelectorAll("article")).toHaveLength(4);
  });

  it("項目の無いグループは空の状態を出し、「Play」を出さない", async () => {
    server.groups.set(3, {
      group: group(3, "Empty", { videoCount: 0, videoIds: [], durationMs: undefined }),
      items: [],
    });
    renderPage("/groups/3");
    expect(await screen.findByText("No items yet")).toBeDefined();
    expect(screen.getByText("0 videos")).toBeDefined();
    expect(screen.queryByRole("link", { name: /^Play/ })).toBeNull();
  });

  it("キーボードで項目を動かすと、上にある項目で move を送り、読み直しても新しい順で出る", async () => {
    seed();
    layoutRows();
    const user = userEvent.setup();
    await openList(user);
    expect(listTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);

    await moveFirstDown(user);

    await waitFor(() =>
      expect(server.writes).toEqual([
        { method: "POST", path: "/api/groups/1/items/101/move", body: { after: 102 } },
      ]),
    );
    expect(listTitles()).toEqual(["Extras", "Episode 1", "Episode 2"]);

    cleanup();
    renderPage("/groups/1");
    await screen.findByRole("table", { name: "Items" });
    expect(listTitles()).toEqual(["Extras", "Episode 1", "Episode 2"]);
  });

  it("グリッドにはハンドルも × も無い", async () => {
    seed();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    expect(screen.queryByRole("button", { name: /^Reorder/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /from the group$/ })).toBeNull();
  });

  it("並べ替えの失敗は最後に認めた順へ戻して知らせる", async () => {
    seed();
    layoutRows();
    const user = userEvent.setup();
    await openList(user);
    const fetchMock = vi.mocked(fetch);
    const original = fetchMock.getMockImplementation() as typeof fetch;
    fetchMock.mockImplementation((input, init) =>
      String(input).endsWith("/move")
        ? Promise.resolve(json({ code: "internal", message: "x" }, 500))
        : original(input, init),
    );

    await moveFirstDown(user);

    expect(await screen.findByText(/^Couldn't move the item:/)).toBeDefined();
    expect(listTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
  });

  it("行の × は確認なしに外し、件数を変え、フォーカスを次の行の × へ移す", async () => {
    seed();
    const user = userEvent.setup();
    await openList(user);
    await user.click(
      screen.getByRole("button", { name: 'Remove "Episode 1" from the group' }),
    );

    await waitFor(() => expect(listTitles()).toEqual(["Extras", "Episode 2"]));
    expect(server.writes).toEqual([
      { method: "POST", path: "/api/groups/1/items/remove", body: { itemIds: [101] } },
    ]);
    expect(screen.getByText("2 videos · 1:32:10")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement?.getAttribute("aria-label")).toBe(
        'Remove "Extras" from the group',
      ),
    );
  });

  it("最後の項目を外したグループは、読み直しが 404 でも空の状態のまま", async () => {
    seed();
    const user = userEvent.setup();
    await openList(user, "/groups/5?trail=1");
    await user.click(
      screen.getByRole("button", { name: 'Remove "Bonus" from the group' }),
    );
    expect(await screen.findByText("No items yet")).toBeDefined();
    expect(screen.getByRole("button", { name: "More" })).toBeDefined();

    // 見える動画の無いグループは 404 になる（plan.md）。外した後の知らせの読み直しでも
    // 「無くなった」とは見せない。
    server.groups.delete(5);
    await emitServerEvent("group", { ids: [1, 5] });
    await waitFor(() => expect(server.reads.get(5)).toBe(2));
    expect(screen.getByText("No items yet")).toBeDefined();
    expect(screen.queryByText("This group no longer exists")).toBeNull();
  });

  it("選んだ 2 項目を「Remove from group」で外し、件数を知らせて選択を外す", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "Extras"' }));
    await user.click(screen.getByRole("button", { name: "Remove from group" }));

    expect(await screen.findByText('Removed 2 items from "Season"')).toBeDefined();
    expect(server.writes).toEqual([
      {
        method: "POST",
        path: "/api/groups/1/items/remove",
        body: { itemIds: [101, 102] },
      },
    ]);
    expect(
      Array.from(document.querySelectorAll("article h3")).map((each) => each.textContent),
    ).toEqual(["Episode 2"]);
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });

  it("対象が途中で消えていたら、編集の失敗を知らせてグループを読み直す", async () => {
    seed();
    const user = userEvent.setup();
    await openList(user);
    // 別のタブが Episode 2 を外した。
    const stored = server.groups.get(1) as StoredGroup;
    stored.items = stored.items.filter((item) => item.id !== 103);
    summarize(stored);

    await user.click(
      screen.getByRole("button", { name: 'Remove "Episode 2" from the group' }),
    );
    expect(
      await screen.findByText("The group changed in the meantime, so it was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(listTitles()).toEqual(["Episode 1", "Extras"]));
    expect(server.reads.get(1)).toBe(2);
  });

  it("改名は前後を削った名前を送り、空では送れない", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await openMenu(user, "Rename…");

    const dialog = await screen.findByRole("dialog", { name: "Rename group" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    expect(within(dialog).getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      true,
    );
    await user.clear(field);
    await user.type(field, "   ");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(server.writes).toEqual([]);

    await user.clear(field);
    await user.type(field, "  Bonus clips  ");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Bonus clips" }),
    ).toBeDefined();
    expect(server.writes).toEqual([
      { method: "PATCH", path: "/api/groups/5", body: { name: "Bonus clips" } },
    ]);
    expect(crumbs()).toEqual(["Library", "Season", "Bonus clips"]);
    expect(document.title).toBe("Bonus clips");
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "More" })),
    );
  });

  it("解散はまず確かめ、確定すると道筋の親へ移ってトーストを出す", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await openMenu(user, "Dissolve…");

    const dialog = await screen.findByRole("alertdialog", { name: 'Dissolve "Extras"?' });
    expect(
      within(dialog).getByText(
        "The group is dissolved. Its videos and groups stay in the library.",
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(server.writes).toEqual([]);

    await openMenu(user, "Dissolve…");
    await user.click(await screen.findByRole("button", { name: "Dissolve" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(server.writes).toEqual([{ method: "DELETE", path: "/api/groups/5" }]);
    expect(screen.getByText('Dissolved "Extras"')).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season"]);
  });

  it("道筋の無い自動グループの解散は 2 文目を足し、ライブラリへ移る", async () => {
    seed();
    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, folder: { rootId: 1, path: "anime/season" } };
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await openMenu(user, "Dissolve…");
    const dialog = await screen.findByRole("alertdialog", { name: 'Dissolve "Season"?' });
    expect(within(dialog).getByText(/Choose Automatic on the folder page/)).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Dissolve" }));
    expect(await screen.findByText("Library")).toBeDefined();
  });

  it("タグに変えると確かめてから送り、道筋の親へ移って結果を知らせる", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await openMenu(user, "Turn the group into a tag…");
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Turn "Extras" into a tag?',
    });
    await user.click(within(dialog).getByRole("button", { name: "Turn into a tag" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(
      screen.getByText('Created the tag "Extras" and dissolved the group'),
    ).toBeDefined();
    expect(server.writes).toEqual([{ method: "POST", path: "/api/groups/5/tag" }]);
  });

  it("ゲストのリスト表示にはハンドルも × も無い", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1", "guest");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("radio", { name: "List" }));
    expect(screen.queryByRole("button", { name: /^Reorder/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /from the group$/ })).toBeNull();
  });

  it("画面の文言はカタログか利用者のデータだけ", async () => {
    seed();
    enablePseudoLocale();
    renderPage("/groups/5?trail=1");
    await waitFor(() => expect(document.querySelectorAll("article")).toHaveLength(1));
    expectCatalogTextOnly(document.body, ["Season", "Extras", "Bonus", "1:00"]);
  });
});
