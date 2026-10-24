import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FolderListing, Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot } from "../api/listSnapshot";
import { AudienceProvider } from "../auth/audience";
import { ScanProvider } from "../shell/ScanProvider";
import { layoutCards, lift, moveTo, openedMenu, release } from "../testing/cardDrag";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import FolderPage from "./FolderPage";

function video(id: number, title: string): Video {
  return {
    id,
    virtual: false,
    title,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    fileModifiedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const listing: FolderListing = {
  folder: {
    rootId: 3,
    path: "A",
    name: "A",
    rootPath: "/a/movies",
    videoCount: 2,
    folderCount: 0,
    previews: [],
  },
  folders: [],
};

const server = {
  videos: [] as Video[],
  writes: [] as { path: string; body: unknown }[],
  nextGroupId: 70,
  /** Gate rename responses until this promise resolves. */
  holdRename: null as Promise<void> | null,
};

const fetchMock = vi.fn<typeof fetch>((input, init) => {
  const url = String(input);
  const method = init?.method ?? "GET";
  if (method !== "GET") {
    const path = new URL(url, "http://localhost").pathname;
    const body = JSON.parse(String(init?.body)) as unknown;
    server.writes.push({ path, body });
    if (path === "/api/organize/group") {
      const groupId = server.nextGroupId;
      server.nextGroupId += 1;
      return Promise.resolve(
        json({ undoToken: `token-group-${String(groupId)}`, groupId }),
      );
    }
    if (path.startsWith("/api/groups/")) {
      return (server.holdRename ?? Promise.resolve()).then(() => json({}));
    }
    if (path === "/api/organize/bundle") {
      const { videoIds } = body as { videoIds: number[] };
      server.videos = server.videos.filter((each) => !videoIds.includes(each.id));
      return Promise.resolve(json({ undoToken: "token-bundle" }));
    }
  }
  if (url.startsWith("/api/scans/current")) return Promise.resolve(json({}, 404));
  if (url === "/api/media-folders") return Promise.resolve(json([{}]));
  if (url === "/api/folders/3?path=A") return Promise.resolve(json(listing));
  if (url.startsWith("/api/folders/3/videos?")) {
    return Promise.resolve(json({ items: server.videos, total: server.videos.length }));
  }
  return Promise.resolve(json({ code: "not_found", message: "x" }, 404));
});

function renderFolder() {
  render(
    <MemoryRouter initialEntries={["/folders/3/A"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
            <ScanProvider>
              <Routes>
                <Route path="/folders/*" element={<FolderPage />} />
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
    (card) => card.querySelector("h3")?.textContent ?? "",
  );
}

async function open() {
  server.videos = [video(1, "x"), video(2, "y")];
  layoutCards();
  renderFolder();
  await screen.findByRole("link", { name: "y" });
}

async function dropXOnY() {
  lift("x", 0);
  moveTo(1);
  expect(screen.queryByLabelText(/items$/)).toBeNull();
  release(1);
  return openedMenu();
}

beforeEach(() => {
  clearListSnapshot();
  server.writes = [];
  server.nextGroupId = 70;
  server.holdRename = null;
  vi.stubGlobal("fetch", fetchMock);
  installFakeEventSource();
  vi.stubGlobal("scrollTo", vi.fn());
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

describe("FolderPage drag a video onto a video", () => {
  it("「Make a group」は 2 枚を残し、落とした先の下の窓に名前の欄を出す", async () => {
    await open();
    const user = userEvent.setup();
    await user.click(
      within(await dropXOnY()).getByRole("menuitem", { name: "Make a group" }),
    );

    await waitFor(() =>
      expect(server.writes).toEqual([
        {
          path: "/api/organize/group",
          body: { items: [{ kind: "video", id: 1 }], target: 2 },
        },
      ]),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    expect((field as HTMLInputElement).value).toBe("y");
    expect(field.closest('[data-slot="popover-content"]')).not.toBeNull();
    expect(cardTitles()).toEqual(["x", "y"]);
  });

  it("前の名前の変更が答える前に次の「Make a group」の欄が開いたら、前の答えはその欄を閉じない", async () => {
    await open();
    let answerRename = () => undefined as void;
    server.holdRename = new Promise<void>((resolve) => {
      answerRename = resolve;
    });
    const user = userEvent.setup();
    await user.click(
      within(await dropXOnY()).getByRole("menuitem", { name: "Make a group" }),
    );
    const first = await screen.findByRole("textbox", { name: "Group name" });
    await user.clear(first);
    await user.type(first, "Trip{Enter}");
    await waitFor(() =>
      expect(screen.queryByRole("textbox", { name: "Group name" })).toBeNull(),
    );

    await user.click(
      within(await dropXOnY()).getByRole("menuitem", { name: "Make a group" }),
    );
    expect(
      ((await screen.findByRole("textbox", { name: "Group name" })) as HTMLInputElement)
        .value,
    ).toBe("y");
    await act(async () => {
      answerRename();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(screen.getByRole("textbox", { name: "Group name" })).toBeDefined();
    expect(server.writes.map((write) => write.path)).toEqual([
      "/api/organize/group",
      "/api/groups/70",
      "/api/organize/group",
    ]);
  });

  it("「Bundle as versions」は持ち上げたカードを消す", async () => {
    await open();
    await userEvent
      .setup()
      .click(
        within(await dropXOnY()).getByRole("menuitem", { name: "Bundle as versions" }),
      );

    expect(cardTitles()).toEqual(["y"]);
    await waitFor(() =>
      expect(server.writes).toEqual([
        { path: "/api/organize/bundle", body: { videoIds: [1], target: 2 } },
      ]),
    );
  });
});
