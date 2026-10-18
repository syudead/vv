import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot, takeListSnapshot } from "../api/listSnapshot";
import { __resetTagsForTest } from "../api/tags";
import { AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { dropOn, layoutCards, openedMenu } from "../testing/cardDrag";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import LibraryPage from "./LibraryPage";

vi.mock("../home/HomeShelves", () => ({ default: () => null }));

function video(id: number, title: string): Video {
  return {
    id,
    title,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: `/api/videos/${String(id)}/thumbnail`,
    durationMs: 60_000,
    width: 1920,
    height: 1080,
    videoCodec: "h264",
    previewState: "pending",
    tags: [],
    favorite: false,
  };
}

function group(id: number, name: string, videoIds: number[]): LibraryGroup {
  return {
    id,
    name,
    videoCount: videoIds.length,
    watchedCount: 0,
    watchState: "unwatched",
    sizeBytes: 1024,
    addedAt: "2026-09-02T00:00:00Z",
    previews: [],
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

interface Write {
  method: string;
  path: string;
  body: unknown;
}

const server = {
  items: [] as LibraryItem[],
  writes: [] as Write[],
  tooManyVideos: false,
  before: [] as LibraryItem[],
  holdBundle: null as Promise<void> | null,
  holdGroup: null as Promise<void> | null,
  nextGroupId: 90,
  failGroupOn: null as number | null,
};

function organize(path: string, body: unknown): Response | undefined {
  if (path === "/api/organize/group") {
    const { items, target } = body as { items: { id: number }[]; target: number };
    if (target === server.failGroupOn)
      return json({ code: "internal", message: "x" }, 500);
    const lifted = new Set(items.map((each) => each.id));
    const targetItem = server.items.find(
      (item) => item.kind === "video" && item.video.id === target,
    );
    const name = targetItem?.kind === "video" ? targetItem.video.title : "";
    const groupId = server.nextGroupId;
    server.nextGroupId += 1;
    const made = group(groupId, name, [target, ...lifted]);
    server.before = server.items;
    server.items = server.items.flatMap((item): LibraryItem[] => {
      if (item.kind !== "video") return [item];
      if (item.video.id === target) return [{ kind: "group", group: made }];
      return lifted.has(item.video.id) ? [] : [item];
    });
    return json({ undoToken: `token-group-${String(groupId)}`, groupId });
  }
  if (path === "/api/organize/bundle" && server.tooManyVideos) {
    return json(
      { code: "invalid_request", reason: "too_many_videos", limit: 20, message: "x" },
      400,
    );
  }
  if (path === "/api/organize/bundle") {
    const { videoIds } = body as { videoIds: number[] };
    server.items = server.items.filter(
      (item) => item.kind !== "video" || !videoIds.includes(item.video.id),
    );
    return json({ undoToken: "token-bundle" });
  }
  if (path === "/api/organize/undo") {
    server.items = server.before;
    return new Response(null, { status: 204 });
  }
  const renamed = /^\/api\/groups\/(\d+)$/.exec(path);
  if (renamed !== null) {
    const id = Number(renamed[1]);
    const { name } = body as { name: string };
    server.items = server.items.map((item) =>
      item.kind === "group" && item.group.id === id
        ? { kind: "group", group: { ...item.group, name } }
        : item,
    );
    return json(group(id, name, []));
  }
  return undefined;
}

function heldAnswer(path: string): Promise<void> | null {
  if (path === "/api/organize/bundle") {
    const hold = server.holdBundle;
    server.holdBundle = null;
    return hold;
  }
  if (path === "/api/organize/group") {
    const hold = server.holdGroup;
    server.holdGroup = null;
    return hold;
  }
  return null;
}

const fetchMock = vi.fn<typeof fetch>((input, init) => {
  const url = new URL(String(input), "http://localhost");
  const method = init?.method ?? "GET";
  if (method !== "GET") {
    const body = JSON.parse(String(init?.body)) as unknown;
    server.writes.push({ method, path: url.pathname, body });
    const hold = heldAnswer(url.pathname);
    const answer = organize(url.pathname, body);
    if (answer !== undefined) return (hold ?? Promise.resolve()).then(() => answer);
  }
  if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
  if (url.pathname === "/api/media-folders") return Promise.resolve(json([{}]));
  if (url.pathname === "/api/processing") {
    return Promise.resolve(
      json({ probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 }),
    );
  }
  if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
  if (url.pathname === "/api/library") {
    return Promise.resolve(json({ items: server.items, total: server.items.length }));
  }
  throw new Error(`unexpected request: ${method} ${url.toString()}`);
});

function renderLibrary() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
            <ScanProvider>
              <Routes>
                <Route path="/" element={<LibraryPage />} />
              </Routes>
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) =>
      card.querySelector("h3")?.textContent ?? card.querySelector("input")?.value ?? "",
  );
}

async function open() {
  server.items = [
    { kind: "video", video: video(1, "A") },
    { kind: "video", video: video(2, "B") },
    { kind: "video", video: video(3, "C") },
  ];
  layoutCards();
  renderLibrary();
  await screen.findByRole("link", { name: "C" });
}

async function dropAOnB() {
  dropOn("A", 0, 1);
  return openedMenu();
}

beforeEach(() => {
  __resetTagsForTest();
  clearListSnapshot();
  localStorage.clear();
  server.writes = [];
  server.tooManyVideos = false;
  server.before = [];
  server.holdBundle = null;
  server.holdGroup = null;
  server.nextGroupId = 90;
  server.failGroupOn = null;
  vi.stubGlobal("fetch", fetchMock);
  installFakeEventSource();
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
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("LibraryPage drag a video onto a video", () => {
  it("A を B に落とすと「Dropped on B」に 2 つの項目を出し、「Make a group」は B の位置に B の名前のグループを 1 枚出す", async () => {
    await open();
    const menu = await dropAOnB();

    expect(within(menu).getByText('Dropped on "B"')).toBeDefined();
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["Make a group", "Bundle as versions"]);
    await userEvent
      .setup()
      .click(within(menu).getByRole("menuitem", { name: "Make a group" }));

    expect(cardTitles()).toEqual(["B", "C"]);
    await waitFor(() =>
      expect(server.writes).toEqual([
        {
          method: "POST",
          path: "/api/organize/group",
          body: { items: [{ kind: "video", id: 1 }], target: 2 },
        },
      ]),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    expect((field as HTMLInputElement).value).toBe("B");
    expect(document.activeElement).toBe(field);
    expect(await screen.findByText('Made the group "B"')).toBeDefined();
  });

  it("前の「Make a group」の答えが後から届いても、後に落とした C と D のグループと名前の欄はそのまま", async () => {
    server.items = ["A", "B", "C", "D"].map((title, index): LibraryItem => ({
      kind: "video",
      video: video(index + 1, title),
    }));
    layoutCards();
    renderLibrary();
    await screen.findByRole("link", { name: "D" });
    let answerFirst = () => undefined as void;
    server.holdGroup = new Promise<void>((resolve) => {
      answerFirst = resolve;
    });
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    dropOn("C", 1, 2);
    await user.click(
      within(await openedMenu()).getByRole("menuitem", { name: "Make a group" }),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    expect((field as HTMLInputElement).value).toBe("D");

    await act(async () => {
      answerFirst();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const kept = screen.getByRole("textbox", { name: "Group name" });
    expect((kept as HTMLInputElement).value).toBe("D");
    expect(screen.getByText('Made the group "D"')).toBeDefined();
    expect(screen.queryByText('Made the group "B"')).toBeNull();
    await waitFor(() => expect(cardTitles()).toEqual(["B", "D"]));
    await user.clear(kept);
    await user.type(kept, "Trip{Enter}");
    await waitFor(() =>
      expect(server.writes.at(-1)).toEqual({
        method: "PATCH",
        path: "/api/groups/91",
        body: { name: "Trip", undoToken: "token-group-91" },
      }),
    );
  });

  it("前の「Make a group」が答えた後に後の操作が失敗したら、前のグループの名前の欄を開く", async () => {
    server.items = ["A", "B", "C", "D"].map((title, index): LibraryItem => ({
      kind: "video",
      video: video(index + 1, title),
    }));
    layoutCards();
    renderLibrary();
    await screen.findByRole("link", { name: "D" });
    let answerFirst = () => undefined as void;
    server.holdGroup = new Promise<void>((resolve) => {
      answerFirst = resolve;
    });
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    let answerSecond = () => undefined as void;
    server.holdGroup = new Promise<void>((resolve) => {
      answerSecond = resolve;
    });
    server.failGroupOn = 4;
    dropOn("C", 1, 2);
    await user.click(
      within(await openedMenu()).getByRole("menuitem", { name: "Make a group" }),
    );

    await act(async () => {
      answerFirst();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(screen.queryByRole("textbox", { name: "Group name" })).toBeNull();

    await act(async () => {
      answerSecond();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(await screen.findByText(/^Couldn't make the group: /)).toBeDefined();
    const field = await screen.findByRole("textbox", { name: "Group name" });
    expect((field as HTMLInputElement).value).toBe("B");
    await user.clear(field);
    await user.type(field, "Trip{Enter}");
    await waitFor(() =>
      expect(server.writes.at(-1)).toEqual({
        method: "PATCH",
        path: "/api/groups/90",
        body: { name: "Trip", undoToken: "token-group-90" },
      }),
    );
  });

  it("名前の欄で変えた名前の Enter は、「Make a group」のトークン付きで名前を変える", async () => {
    await open();
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    await user.clear(field);
    await user.type(field, "Trip{Enter}");

    await waitFor(() =>
      expect(server.writes.at(-1)).toEqual({
        method: "PATCH",
        path: "/api/groups/90",
        body: { name: "Trip", undoToken: "token-group-90" },
      }),
    );
    expect(await screen.findByRole("heading", { name: "Trip" })).toBeDefined();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("名前の欄を開いている間に一覧が読み直されても、欄は入力ごと残る", async () => {
    await open();
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    await user.clear(field);
    await user.type(field, "Tri");
    await emitServerEvent("group", { ids: [90] });
    await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(0));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const kept = screen.getByRole("textbox", { name: "Group name" });
    expect((kept as HTMLInputElement).value).toBe("Tri");
    expect(document.activeElement).toBe(kept);
    expect(cardTitles()).toEqual(["Tri", "C"]);
  });

  it("名前の欄を開いたまま Undo を押すと、欄が閉じて A と B が戻る", async () => {
    await open();
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    await screen.findByRole("textbox", { name: "Group name" });
    fireEvent.click(await screen.findByRole("button", { name: "Undo" }));

    expect(await screen.findByText("Undone")).toBeDefined();
    await waitFor(() =>
      expect(screen.queryByRole("textbox", { name: "Group name" })).toBeNull(),
    );
    await waitFor(() => expect(cardTitles()).toEqual(["A", "B", "C"]));
    expect(server.writes.some((each) => each.method === "PATCH")).toBe(false);
  });

  it("作った直後のカードを押しても古い一覧を控えにせず、読み直した後の一覧を控えにする", async () => {
    await open();
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Make a group" }),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    await user.click(field);
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    await user.keyboard("{Escape}");
    await screen.findByRole("heading", { name: "B" });
    await user.click(screen.getByRole("heading", { name: "C" }));
    const snapshot = takeListSnapshot({ query: "" });
    expect(snapshot?.items.map((item) => item.kind)).toEqual(["group", "video"]);
  });

  it("選んだ 2 枚を落として失敗したら、カードと選択を戻す", async () => {
    await open();
    server.tooManyVideos = true;
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: 'Select "A"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "C"' }));
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Bundle as versions" }),
    );

    expect(
      await screen.findByText("Couldn't bundle: Select between 1 and 20 videos."),
    ).toBeDefined();
    expect(cardTitles()).toEqual(["A", "B", "C"]);
    expect(screen.getByRole("region", { name: "Selection actions" })).toBeDefined();
    expect(
      screen.getByRole("checkbox", { name: 'Select "A"' }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(
      screen.getByRole("checkbox", { name: 'Select "C"' }).getAttribute("aria-checked"),
    ).toBe("true");
  });

  it("「Bundle as versions」は B を代表に束ねて A を消し、ダイアログを開かない", async () => {
    await open();
    await userEvent
      .setup()
      .click(
        within(await dropAOnB()).getByRole("menuitem", { name: "Bundle as versions" }),
      );

    expect(cardTitles()).toEqual(["B", "C"]);
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() =>
      expect(server.writes).toEqual([
        {
          method: "POST",
          path: "/api/organize/bundle",
          body: { videoIds: [1], target: 2 },
        },
      ]),
    );
    expect(await screen.findByText('Bundled 2 videos as versions of "B"')).toBeDefined();
  });

  it("束ねて 20 本を超えると持ち上げたカードを戻し、理由を出して取り消しは出さない", async () => {
    await open();
    server.tooManyVideos = true;
    await userEvent
      .setup()
      .click(
        within(await dropAOnB()).getByRole("menuitem", { name: "Bundle as versions" }),
      );

    expect(
      await screen.findByText("Couldn't bundle: Select between 1 and 20 videos."),
    ).toBeDefined();
    expect(cardTitles()).toEqual(["A", "B", "C"]);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("画面を離れた後に答えた要求は、取り消しの通知を出さない", async () => {
    await open();
    let answer = () => undefined as void;
    server.holdBundle = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const user = userEvent.setup();
    await user.click(
      within(await dropAOnB()).getByRole("menuitem", { name: "Bundle as versions" }),
    );
    await waitFor(() =>
      expect(server.writes.some((each) => each.path === "/api/organize/bundle")).toBe(
        true,
      ),
    );
    await user.click(screen.getByRole("link", { name: "C" }));
    await waitFor(() => expect(screen.queryByRole("link", { name: "C" })).toBeNull());
    await act(async () => {
      answer();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    expect(screen.queryByText(/Bundled 2 videos/)).toBeNull();
  });

  it("Esc でメニューを閉じると何も送らず、A と B はそのまま", async () => {
    await open();
    await dropAOnB();
    await userEvent.setup().keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(cardTitles()).toEqual(["A", "B", "C"]);
    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
  });
});
