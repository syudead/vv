import { act, render, screen, waitFor, within } from "@testing-library/react";
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

function install() {
  const fetchMock = vi.fn<typeof fetch>((input) => {
    const url = new URL(String(input), "http://localhost");
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
  vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
  install();
  installFakeEventSource();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

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

  it("画面の文言はカタログか利用者のデータだけ", async () => {
    seed();
    enablePseudoLocale();
    renderPage("/groups/5?trail=1");
    await waitFor(() => expect(document.querySelectorAll("article")).toHaveLength(1));
    expectCatalogTextOnly(document.body, ["Season", "Extras", "Bonus", "1:00"]);
  });
});
