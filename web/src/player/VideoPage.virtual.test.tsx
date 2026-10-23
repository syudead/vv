import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { setRenderedAudience as forgetAudience, type Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import VideoDetailsEditor from "./detailsEditor/VideoDetailsEditor";
import VideoPage from "./VideoPage";

const link = "https://example.com/watch?v=1";

const virtualVideo: Video = {
  id: 7,
  virtual: true,
  title: "仮の動画",
  public: true,
  sizeBytes: 0,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-02T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  playable: false,
  probeState: "pending",
  thumbnailState: "pending",
  previewState: "pending",
  contentVersion: "virtual:3",
  favorite: false,
  tags: [],
  workInfo: {
    description: "配信の告知",
    dates: [],
    people: [],
    organizations: [],
    urls: [link],
    externalIds: [],
  },
};

// Unplayable keeps the real player out of tests that only land on a file video's page.
const mergeTarget: Video = {
  ...virtualVideo,
  id: 9,
  virtual: false,
  title: "ファイルの動画",
  playable: false,
  probeState: "done",
  contentVersion: "abc",
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function failure(status: number, code: string, reason: string): Response {
  return json({ code, reason, message: reason }, status);
}

const server = {
  video: virtualVideo,
  details: vi.fn<(body: unknown) => Response>(),
  remove: vi.fn<(url: URL) => Response>(),
  requests: [] as string[],
};

function renderPage(audience: Audience = "owner") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter
          initialEntries={[{ pathname: "/videos/7", state: { from: "/library" } }]}
        >
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
              <Route path="/videos/:id/edit" element={<VideoDetailsEditor />} />
              <Route path="/library" element={<p>Library list</p>} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

async function openPage(audience: Audience = "owner") {
  renderPage(audience);
  await screen.findByRole("heading", { level: 1, name: server.video.title });
}

describe("VideoPage for a virtual video", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    server.video = virtualVideo;
    server.details.mockReset();
    server.remove.mockReset();
    server.requests = [];
    installFakeEventSource();
    forgetAudience(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      server.requests.push(`${method} ${url.split("?")[0]!}`);
      const body: unknown =
        typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
      if (url === "/api/auth/session") return Promise.resolve(json({ state: "owner" }));
      if (url === "/api/videos/7/details" && method === "PUT")
        return Promise.resolve(server.details(body));
      if (url.startsWith("/api/videos/7?") && method === "DELETE")
        return Promise.resolve(server.remove(new URL(url, "http://localhost")));
      if (url === "/api/videos/7") return Promise.resolve(json(server.video));
      if (url.startsWith("/api/videos?"))
        return Promise.resolve(json({ items: [mergeTarget], total: 1 }));
      if (url === "/api/virtual-videos/7/merge" && method === "POST")
        return Promise.resolve(json(mergeTarget));
      if (url === "/api/videos/9") return Promise.resolve(json(mergeTarget));
      if (url === "/api/videos/7/related" || url === "/api/videos/9/related")
        return Promise.resolve(json({ items: [] }));
      if (url === "/api/videos/7/subtitles")
        return Promise.resolve(json({ subtitles: [] }));
      if (url === "/api/tags") return Promise.resolve(json([]));
      if (url.startsWith("/api/people") || url.startsWith("/api/organizations"))
        return Promise.resolve(json({ items: [], total: 0, totalAll: 0 }));
      return Promise.resolve(json({}));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("プレーヤーの代わりにサムネイルの枠を置き、リンクは新しいタブで開くボタンにする", async () => {
    await openPage();
    expect(screen.getByText("Virtual video")).toBeDefined();
    expect(screen.getByText("No thumbnail yet")).toBeDefined();
    expect(screen.queryByText("Getting ready to play")).toBeNull();
    const links = screen.getByRole("list", { name: "Links" });
    const anchor = within(links).getByRole("link", { name: "example.com/watch?v=1" });
    expect(anchor.getAttribute("href")).toBe(link);
    expect(anchor.getAttribute("target")).toBe("_blank");
    expect(anchor.getAttribute("rel")).toBe("noreferrer");
    expect(screen.getByText("配信の告知")).toBeDefined();
    // The work details row does not repeat what the links and description show.
    expect(screen.queryByRole("list", { name: "Work details" })).toBeNull();
    const facts = within(screen.getByRole("list", { name: "File details" }));
    expect(facts.queryByText("Size")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Use current frame as thumbnail" }),
    ).toBeNull();
  });

  it("ゲストには作成、編集、マージ、削除の操作が無く、事実の行は Added だけ", async () => {
    await openPage("guest");
    expect(screen.getByText("No thumbnail")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Upload an image…" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit name" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Edit details" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Change thumbnail" })).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    expect(screen.queryByRole("switch")).toBeNull();
    const facts = screen.getByRole("list", { name: "File details" });
    expect(within(facts).getAllByRole("listitem")).toHaveLength(1);
    expect(within(facts).getByRole("button", { name: /^Added / })).toBeDefined();
    expect(
      within(screen.getByRole("list", { name: "Links" })).getAllByRole("link"),
    ).toHaveLength(1);
  });

  it("Change thumbnail はメニューを開かずにファイルを選ばせる", async () => {
    const click = vi
      .spyOn(HTMLInputElement.prototype, "click")
      .mockImplementation(() => {});
    await openPage();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Change thumbnail" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(click).toHaveBeenCalledTimes(1);
    expect(click.mock.contexts[0]).toBe(screen.getByTestId("thumbnail-upload-input"));
  });

  it("空の題名は送らずに Enter a title. を出す", async () => {
    await openPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Edit name" }));
    await user.clear(screen.getByRole("textbox", { name: "Display name" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByRole("alert").textContent).toContain("Enter a title.");
    expect(server.requests).not.toContain("PUT /api/videos/7/display-name");
  });

  it("詳細編集の保存に 404 が返ると、作り直さずに動画が無くなったことを伝える", async () => {
    server.details.mockReturnValue(failure(404, "not_found", "video_not_found"));
    await openPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("link", { name: "Edit details" }));
    const title = await screen.findByRole("textbox", { name: "Title" });
    await user.clear(title);
    await user.type(title, "新しい題名");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(
      await screen.findByText(
        "This virtual video was deleted or merged. Nothing was saved.",
      ),
    ).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Back to the library" }).getAttribute("href"),
    ).toBe("/");
    expect(server.details).toHaveBeenCalledTimes(1);
    expect(server.details.mock.calls[0]![0]).toMatchObject({
      displayName: "新しい題名",
      contentVersion: "virtual:3",
    });
    expect(server.requests.filter((r) => r.startsWith("POST"))).toEqual([]);
  });

  it("削除は読んだ版を送り、成功すると戻り先へ閉じてトーストを出す", async () => {
    server.remove.mockReturnValue(new Response(null, { status: 204 }));
    await openPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "仮の動画"?' });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await screen.findByText("Library list")).toBeDefined();
    expect(await screen.findByText('Deleted "仮の動画"')).toBeDefined();
    expect(server.remove.mock.calls[0]![0].searchParams.get("contentVersion")).toBe(
      "virtual:3",
    );
  });

  it("More のマージは Delete… の区切りの前にあり、成功すると同じ戻り先でファイルのある動画のページに置き換わる", async () => {
    await openPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    const menu = await screen.findByRole("menu");
    expect(
      [...menu.querySelectorAll('[role="menuitem"], [role="separator"]')].map(
        (item) => item.textContent || item.getAttribute("role"),
      ),
    ).toEqual(["Merge into a file video…", "separator", "Delete…"]);
    await user.click(
      within(menu).getByRole("menuitem", { name: "Merge into a file video…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Merge into a file video" });
    await user.click(
      await within(dialog).findByRole("option", { name: /ファイルの動画/ }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "ファイルの動画" }),
    ).toBeDefined();
    expect(screen.getByText('Merged into "ファイルの動画"')).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(await screen.findByText("Library list")).toBeDefined();
  });

  it("削除に 409 content_changed が返ると、ダイアログに残して Reload で読み直す", async () => {
    server.remove.mockReturnValue(failure(409, "conflict", "content_changed"));
    await openPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(
      await within(dialog).findByText(
        "This video changed in another tab. Reload and try again.",
      ),
    ).toBeDefined();
    const reads = server.requests.filter((r) => r === "GET /api/videos/7").length;
    await user.click(within(dialog).getByRole("button", { name: "Reload" }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(server.requests.filter((r) => r === "GET /api/videos/7").length).toBe(
      reads + 1,
    );
  });
});

describe("VideoPage for a file video, as a guest", () => {
  beforeEach(() => {
    installFakeEventSource();
    forgetAudience(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("作品情報の行に説明とリンクを出す", async () => {
    const fileVideo: Video = {
      ...virtualVideo,
      virtual: false,
      title: "ファイルの動画",
      // Unplayable keeps the real player out of this test; the details below are the subject.
      playable: false,
      probeState: "done",
      contentVersion: undefined,
      favorite: undefined,
    };
    server.video = fileVideo;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input) => {
        const url = String(input);
        if (url === "/api/videos/7") return Promise.resolve(json(fileVideo));
        if (url === "/api/videos/7/related") return Promise.resolve(json({ items: [] }));
        return Promise.resolve(json({}));
      }),
    );
    await openPage("guest");
    const row = screen.getByRole("list", { name: "Work details" });
    expect(
      within(row)
        .getByRole("link", { name: /example\.com/ })
        .getAttribute("target"),
    ).toBe("_blank");
    expect(screen.getByText("配信の告知")).toBeDefined();
  });
});
