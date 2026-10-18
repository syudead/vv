import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { OrganizableItem as LibraryItem } from "../api/libraryItems";
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

function group(id: number, name: string, videoIds: number[]): LibraryGroup {
  return {
    id,
    name,
    videoCount: videoIds.length,
    watchedCount: 0,
    watchState: "unwatched",
    durationMs: videoIds.length * 60_000,
    sizeBytes: videoIds.length * 1024 * 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: videoIds.map((videoId) => ({
      videoId,
      thumbnailUrl: `/api/videos/${String(videoId)}/thumbnail`,
    })),
    openVideoId: videoIds[0] ?? 0,
    videoIds,
    tags: [],
    favorite: false,
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

interface CreateBody {
  name: string;
  videoIds: number[];
  groupIds: number[];
}

interface AddBody {
  videoIds: number[];
  groupIds: number[];
}

interface Server {
  items: LibraryItem[];
  groups: LibraryGroup[];
  creates: CreateBody[];
  adds: { id: number; body: AddBody }[];
  deletes: unknown[];
  libraryReads: number;
  /** Child-to-parent links model descendant-cycle rejection. */
  parents: Map<number, number>;
}

const fetchMock = vi.fn<typeof fetch>();

function isInside(parents: Map<number, number>, child: number, ancestor: number) {
  for (let current: number | undefined = child; current !== undefined;) {
    if (current === ancestor) return true;
    current = parents.get(current);
  }
  return false;
}

function createGroupIn(server: Server, body: CreateBody): Response {
  const memberVideos = new Set(body.videoIds);
  const memberGroups = new Set(body.groupIds);
  const created = group(90 + server.creates.length, body.name.trim(), body.videoIds);
  server.creates.push(body);
  server.groups = [...server.groups, created];
  server.items = [
    ...server.items.filter((item) =>
      item.kind === "video"
        ? !memberVideos.has(item.video.id)
        : !memberGroups.has(item.group.id),
    ),
    { kind: "group", group: created },
  ];
  return json(created, 201);
}

function addToGroupIn(server: Server, id: number, body: AddBody): Response {
  if (body.groupIds.some((groupId) => isInside(server.parents, id, groupId))) {
    return json({ code: "conflict", reason: "group_cycle", message: "cycle" }, 409);
  }
  server.adds.push({ id, body });
  for (const groupId of body.groupIds) server.parents.set(groupId, id);
  server.items = server.items.filter((item) =>
    item.kind === "video"
      ? !body.videoIds.includes(item.video.id)
      : !body.groupIds.includes(item.group.id),
  );
  const target = server.groups.find((candidate) => candidate.id === id);
  return json(target, 200);
}

function install(items: LibraryItem[], groups: LibraryGroup[] = []): Server {
  const server: Server = {
    items,
    groups,
    creates: [],
    adds: [],
    deletes: [],
    libraryReads: 0,
    parents: new Map(),
  };
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
      server.libraryReads += 1;
      return Promise.resolve(json({ items: server.items, total: server.items.length }));
    }
    if (url.pathname === "/api/groups" && method === "GET") {
      return Promise.resolve(json({ groups: server.groups }));
    }
    if (url.pathname === "/api/groups" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as CreateBody;
      return Promise.resolve(createGroupIn(server, body));
    }
    const add = /^\/api\/groups\/(\d+)\/items$/.exec(url.pathname);
    if (add !== null && method === "POST") {
      const body = JSON.parse(String(init?.body)) as AddBody;
      return Promise.resolve(addToGroupIn(server, Number(add[1]), body));
    }
    if (url.pathname === "/api/collections/delete" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { groupIds: number[] };
      server.deletes.push(body);
      server.items = server.items.flatMap((item) =>
        item.kind === "group" && body.groupIds.includes(item.group.id)
          ? item.group.videoIds.map((id) => ({
              kind: "video" as const,
              video: video(id),
            }))
          : [item],
      );
      return Promise.resolve(json({ dissolvedGroups: 1, deletedSmartFolders: 0 }));
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

function selectionBar() {
  return screen.getByRole("region", { name: "Selection actions" });
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

describe("ライブラリからグループを作る・足す", () => {
  it("選んだ 3 本と名前でグループができ、3 枚の代わりにそのカードが出て、選択は外れる（受け入れ条件 1）", async () => {
    const server = install([
      { kind: "video", video: video(1) },
      { kind: "video", video: video(2) },
      { kind: "video", video: video(3) },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    for (const id of [1, 2, 3]) {
      await user.click(
        await screen.findByRole("checkbox", { name: `Select "動画 ${String(id)}"` }),
      );
    }

    const names = within(selectionBar())
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));
    const playlist = names.indexOf("Add to playlist");
    expect(names.slice(playlist, playlist + 4)).toEqual([
      "Add to playlist",
      "New group",
      "Add to group",
      "Visibility",
    ]);

    await user.click(within(selectionBar()).getByRole("button", { name: "New group" }));
    const dialog = await screen.findByRole("dialog", { name: "New group" });
    expect(within(dialog).getByText("3 videos become one group")).toBeDefined();
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    await waitFor(() => expect(document.activeElement).toBe(field));

    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(server.creates).toEqual([]);

    await user.type(field, "  Trip  {Enter}");
    expect(await screen.findByText('Created "Trip"')).toBeDefined();
    expect(server.creates).toEqual([{ name: "Trip", videoIds: [1, 2, 3], groupIds: [] }]);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      await screen.findByRole("link", { name: /^Trip, group of 3 videos/ }),
    ).toBeDefined();
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "動画 1" })).toBeNull(),
    );
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });

  it("グループ 1 つだけの選択では「New group」を押せず、理由を添える。動画を足すと押せる", async () => {
    install([
      { kind: "video", video: video(1) },
      { kind: "group", group: group(7, "series", [101, 102]) },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    const newGroup = within(selectionBar()).getByRole("button", { name: "New group" });
    expect((newGroup as HTMLButtonElement).disabled).toBe(true);
    expect(newGroup.getAttribute("title")).toBe(
      "Select at least one video, or a group and something else",
    );

    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
    expect((newGroup as HTMLButtonElement).disabled).toBe(false);
    await user.click(newGroup);
    expect(await screen.findByText("1 video and 1 group become one group")).toBeDefined();
  });

  it("選んだグループを別のグループに足すと、その id を送り、選択は残る。ピッカーは選んだグループ自身を出さない", async () => {
    const series = group(7, "series", [101, 102]);
    const extras = group(8, "extras", [201]);
    const server = install(
      [
        { kind: "group", group: series },
        { kind: "group", group: extras },
      ],
      [extras, series],
    );
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    await user.click(
      within(selectionBar()).getByRole("button", { name: "Add to group" }),
    );
    const list = await screen.findByRole("listbox", { name: "Groups" });
    expect(within(list).queryByRole("option", { name: "series" })).toBeNull();
    const reads = server.libraryReads;
    await user.click(within(list).getByRole("option", { name: "extras" }));

    expect(await screen.findByText('Added to "extras"')).toBeDefined();
    expect(server.adds).toEqual([{ id: 8, body: { videoIds: [], groupIds: [7] } }]);
    expect(screen.queryByRole("listbox", { name: "Groups" })).toBeNull();
    await waitFor(() => expect(server.libraryReads).toBeGreaterThan(reads));
    expect(screen.getByText("2 videos selected")).toBeDefined();
  });

  it("B をその子に足そうとすると、入れられないと言い、ピッカーは開いたまま（受け入れ条件 3）", async () => {
    const parent = group(7, "B", [101]);
    const child = group(8, "A", [201]);
    const server = install([{ kind: "group", group: parent }], [child, parent]);
    server.parents.set(8, 7);
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("button", { name: 'Add group "B" to a group' }),
    );
    await user.click(await screen.findByRole("option", { name: "A" }));

    expect(
      await screen.findByText(
        "Couldn't add to the group: A group can't be put inside itself or inside a group it contains.",
      ),
    ).toBeDefined();
    expect(screen.getByRole("listbox", { name: "Groups" })).toBeDefined();
    expect(server.adds).toEqual([]);
  });

  it("カードの印は「Add to playlist」の左にあり、打った名前に合うグループが無ければ作成の行で作る", async () => {
    const server = install(
      [{ kind: "video", video: video(1) }],
      [group(8, "extras", [201])],
    );
    const user = userEvent.setup();
    renderLibrary();
    const trigger = await screen.findByRole("button", {
      name: 'Add "動画 1" to a group',
    });
    const playlist = screen.getByRole("button", { name: 'Add "動画 1" to a playlist' });
    expect(trigger.nextElementSibling).toBe(playlist);
    expect(trigger.className).toContain("opacity-0");
    expect(trigger.className).toContain("size-8");

    await user.click(trigger);
    const field = await screen.findByRole("combobox", {
      name: "Search or create a group",
    });
    await screen.findByRole("option", { name: "extras" });
    await user.type(field, "Night");
    const options = within(screen.getByRole("listbox", { name: "Groups" })).getAllByRole(
      "option",
    );
    expect(options.map((option) => option.textContent)).toEqual(['Create "Night"']);
    await user.click(options[0]!);

    expect(await screen.findByText('Created "Night"')).toBeDefined();
    expect(server.creates).toEqual([{ name: "Night", videoIds: [1], groupIds: [] }]);
    expect(screen.queryByText("再生画面")).toBeNull();
  });

  it("リスト表示では「Add to playlist」の列の直後の列に置き、sm 未満では隠す", async () => {
    useListView();
    install([{ kind: "video", video: video(1) }]);
    renderLibrary();
    const trigger = await screen.findByRole("button", {
      name: 'Add "動画 1" to a group',
    });
    const cell = trigger.closest("td") as HTMLElement;
    expect(
      within(cell.previousElementSibling as HTMLElement).getByRole("button", {
        name: 'Add "動画 1" to a playlist',
      }),
    ).toBeDefined();
    const row = cell.closest("tr") as HTMLElement;
    const heads = screen.getAllByRole("columnheader");
    expect(row.children).toHaveLength(heads.length);
    const head = heads[Array.from(row.children).indexOf(cell)] as HTMLElement;
    for (const element of [cell, head]) {
      expect(element.className.split(" ")).toEqual(
        expect.arrayContaining(["hidden", "w-8", "sm:table-cell"]),
      );
    }
  });

  it("ゲストには作成と追加の操作が出ない", async () => {
    install([
      { kind: "video", video: video(1, { public: true, favorite: undefined }) },
      {
        kind: "group",
        group: { ...group(7, "series", [101]), favorite: undefined },
      },
    ]);
    renderLibrary("guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: /to a group$/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "New group" })).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
    cleanup();

    useListView();
    renderLibrary("guest");
    await screen.findByRole("link", { name: "動画 1" });
    expect(screen.queryByRole("button", { name: /to a group$/ })).toBeNull();
  });
});

describe("ライブラリで選んだグループを削除する", () => {
  it("確認すると選んだグループだけを送り、選択を外して一覧を読み直す。動画は残る", async () => {
    const server = install([
      { kind: "video", video: video(1) },
      { kind: "group", group: group(7, "series", [101, 102]) },
    ]);
    const user = userEvent.setup();
    renderLibrary();
    await user.click(
      await screen.findByRole("checkbox", { name: 'Select the group "series"' }),
    );
    await user.click(screen.getByRole("checkbox", { name: 'Select "動画 1"' }));
    const reads = server.libraryReads;

    await user.click(within(selectionBar()).getByRole("button", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Dissolve 1 group?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Dissolved 1 group")).toBeDefined();
    expect(server.deletes).toEqual([{ groupIds: [7], smartFolderIds: [] }]);
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
    expect(await screen.findByRole("link", { name: "動画 101" })).toBeDefined();
    expect(server.libraryReads).toBeGreaterThan(reads);
    expect(screen.getByRole("link", { name: "動画 1" })).toBeDefined();
  });
});
