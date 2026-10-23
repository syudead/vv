import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import {
  Link,
  MemoryRouter,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { setRenderedAudience as forgetAudience, type Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import type { WorkInfo } from "../api/workInfo";
import { type Audience, AudienceProvider } from "../auth/audience";
import { formatDate } from "../i18n";
import { ToastProvider } from "../ui/Toast";
import { Button } from "../ui/shadcn/button";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import type { PlayerControls } from "./playerControls";
import VideoPage from "./VideoPage";

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: {
    onControls: (controls: PlayerControls | null) => void;
  }) {
    const [initial] = useState(props);
    useEffect(() => () => initial.onControls(null), [initial]);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: () => true,
  initialPlayerStatus: {
    loading: false,
    reconnecting: false,
    playing: false,
    userActive: true,
    ended: false,
    stalled: false,
    positioned: false,
  },
}));

const work: WorkInfo = {
  code: "KYT-002",
  dates: [{ kind: "release", date: "2024-03-15" }],
  people: [
    {
      role: "performer",
      person: { id: 1, name: "山田 花子", provisional: false },
      provisional: false,
    },
    {
      role: "performer",
      person: { id: 2, name: "佐藤 健太", provisional: false },
      provisional: false,
    },
  ],
  organizations: [],
  urls: [],
  externalIds: [],
};

// Noon keeps the formatted date stable in any time zone.
const added = new Date(2026, 8, 20, 12, 0).toISOString();
const watched = new Date(2026, 9, 2, 12, 0).toISOString();

const owned: Video = {
  id: 7,
  virtual: false,
  title: "テスト動画",
  public: false,
  favorite: false,
  sizeBytes: 84_331_821,
  addedAt: added,
  updatedAt: added,
  fileCreatedAt: added,
  fileModifiedAt: added,
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mkv",
  videoCodec: "h264",
  audioCodec: "aac",
  location: { path: "/media/a/b.mkv", openable: true },
  progress: { positionMs: 1_000, completed: false, updatedAt: watched },
  versions: { count: 3, representativeId: 7 },
  workInfo: work,
  tags: [],
};

const server = {
  videos: new Map<number, Video>(),
};

const approvedWorkPlugin = {
  id: 1,
  kind: "work",
  sources: [],
  fileName: "a.js",
  operations: ["search"],
  digest: "d",
  state: "approved",
  settings: [],
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function respond(url: string): Response {
  if (url === "/api/auth/session") return json({ state: "owner" });
  if (url === "/api/work-info-plugins") return json({ plugins: [] });
  if (url === "/api/tags") return json([]);
  const match = /^\/api\/videos\/(\d+)(\/[a-z-]+)?$/.exec(url);
  if (match === null) return json({});
  const id = Number(match[1]);
  if (match[2] === "/related") return json({ items: [] });
  if (match[2] === "/subtitles") return json({ subtitles: [] });
  if (match[2] !== undefined) return json({});
  const found = server.videos.get(id);
  return found === undefined
    ? json({ code: "not_found", message: "x" }, 404)
    : json(found);
}

function renderPage(audience: Audience = "owner") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/videos/7"]}>
          <AudienceProvider audience={audience}>
            <Link to="/videos/8">Next video</Link>
            <Link to="/videos/9">Third video</Link>
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

async function openPage(audience: Audience = "owner", title = "テスト動画") {
  renderPage(audience);
  await screen.findByRole("heading", { level: 1, name: title });
}

function tabNames(): string[] {
  return screen.getAllByRole("tab").map((tab) => tab.textContent ?? "");
}

function selectedTab(): string | null {
  return screen.getByRole("tab", { selected: true }).textContent;
}

/** Radix tabs select on mouse down, not on click. */
function openTab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name: new RegExp(`^${name}`) }));
}

function facts(): [string, string][] {
  return screen
    .getAllByRole("term")
    .map((term) => [term.textContent ?? "", term.nextElementSibling?.textContent ?? ""]);
}

describe("VideoPage tabs", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    server.videos.clear();
    server.videos.set(7, owned);
    installFakeEventSource();
    forgetAudience(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input) => Promise.resolve(respond(String(input))));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** Hold the work-info plugin list; the returned function answers it. */
  function holdPlugins() {
    let answer: (response: Response) => void = () => undefined;
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url !== "/api/work-info-plugins") return Promise.resolve(respond(url));
      return new Promise<Response>((resolve) => {
        answer = resolve;
      });
    });
    return (plugins: unknown[]) => answer(json({ plugins }));
  }

  it("コード・出演者・3 本のバージョンがある所有者の動画は 6 つのタブを並べて Overview を選び、題名まわりの操作はすべて文字付き（受け入れ条件 1）", async () => {
    await openPage();
    expect(tabNames()).toEqual([
      "Overview",
      "Work",
      "People2",
      "Versions3",
      "Thumbnail",
      "File & media",
    ]);
    expect(selectedTab()).toBe("Overview");
    for (const control of [
      screen.getByRole("button", { name: "Favorite" }),
      screen.getByRole("button", { name: "Add to playlist" }),
      screen.getByRole("button", { name: "Add to group" }),
      screen.getByRole("link", { name: "Edit details" }),
    ]) {
      expect(control.textContent).not.toBe("");
    }
    // The switch's word stands before the track, in the label that holds both.
    const visibility = screen.getByRole("switch", {
      name: "Show to people who aren't signed in",
    });
    expect(visibility.closest("label")?.textContent).toBe("Private");
    expect(
      screen.queryByRole("button", { name: "Use current frame as thumbnail" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy path" })).toBeNull();
  });

  it("Overview は説明・作品の要約・ラベル付きの 4 つの数値を出す（受け入れ条件 2）", async () => {
    server.videos.set(7, { ...owned, workInfo: { ...work, description: "夏の旅" } });
    await openPage();
    const panel = screen.getByRole("tabpanel", { name: "Overview" });
    expect(within(panel).getByText("夏の旅")).toBeDefined();
    expect(
      within(panel).getByText("KYT-002 · Released 2024-03-15 · 山田 花子, 佐藤 健太"),
    ).toBeDefined();
    expect(facts()).toEqual([
      ["Length", "4:02"],
      ["Resolution", "1920×1080"],
      ["Added", formatDate(added)],
      ["Last watched", formatDate(watched)],
    ]);
  });

  it("「Copy path」は所在の絶対パスをクリップボードへ書く（受け入れ条件 6）", async () => {
    const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue();
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    await openPage();
    openTab("File & media");
    expect(screen.getByText("/media/a/b.mkv")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Copy path" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("/media/a/b.mkv"));
  });

  it("ゲストの公開動画は Overview と File & media、2 本なら Versions を並べ、所在・タグ・公開の切り替え・Edit details を出さない（受け入れ条件 8）", async () => {
    // Guest responses omit the location, the progress, the work information and the tags.
    server.videos.set(7, {
      ...owned,
      public: true,
      favorite: undefined,
      location: undefined,
      progress: undefined,
      workInfo: undefined,
      versions: { count: 2, representativeId: 7 },
    });
    await openPage("guest");
    expect(tabNames()).toEqual(["Overview", "Versions2", "File & media"]);
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("link", { name: "Edit details" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Favorite" })).toBeNull();
    expect(screen.queryByRole("combobox", { name: "Add tag" })).toBeNull();
    expect(facts().map(([term]) => term)).toEqual(["Length", "Resolution", "Added"]);

    openTab("File & media");
    expect(screen.queryByRole("region", { name: "Location" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy path" })).toBeNull();
    expect(screen.getByRole("region", { name: "Media" })).toBeDefined();
    expect(screen.getByLabelText("File")).toBeDefined();
  });

  it("Work のある動画から無い動画へ移ると Overview を選び、その次に Work のある動画へ移っても Overview のまま（受け入れ条件 9）", async () => {
    // A code alone lists Work through the availability rule; its panel arrives with its own unit.
    server.videos.set(8, { ...owned, id: 8, title: "後続の動画", workInfo: undefined });
    server.videos.set(9, { ...owned, id: 9, title: "三本目" });
    await openPage();
    openTab("Work");
    expect(selectedTab()).toBe("Work");

    fireEvent.click(screen.getByRole("link", { name: "Next video" }));
    await screen.findByRole("heading", { level: 1, name: "後続の動画" });
    expect(tabNames()).not.toContain("Work");
    expect(selectedTab()).toBe("Overview");

    // The return to Overview is the new selection, not a fallback for this video only.
    fireEvent.click(screen.getByRole("link", { name: "Third video" }));
    await screen.findByRole("heading", { level: 1, name: "三本目" });
    expect(selectedTab()).toBe("Overview");
  });

  describe("アドレスの tab", () => {
    function Address() {
      const location = useLocation();
      return <output data-testid="address">{location.pathname + location.search}</output>;
    }

    function Back() {
      const navigate = useNavigate();
      return (
        <Button variant="ghost" onClick={() => void navigate(-1)}>
          Back
        </Button>
      );
    }

    function renderAt(entry: string) {
      return render(
        <TooltipProvider>
          <ToastProvider>
            <MemoryRouter initialEntries={["/", entry]} initialIndex={1}>
              <AudienceProvider audience="owner">
                <Link to="/">Library</Link>
                <Link to="/videos/7">First video</Link>
                <Link to="/videos/8">Next video</Link>
                <Link to="/videos/8?tab=nope">Next video with an unknown tab</Link>
                <Back />
                <Address />
                <Routes>
                  <Route path="/" element={<p>ライブラリ</p>} />
                  <Route path="/videos/:id" element={<VideoPage />} />
                </Routes>
              </AudienceProvider>
            </MemoryRouter>
          </ToastProvider>
        </TooltipProvider>,
      );
    }

    const address = () => screen.getByTestId("address").textContent;

    it("選んだタブをアドレスに置き換えで書き、ほかの画面へ移って Back で戻ると同じタブを開く", async () => {
      renderAt("/videos/7");
      await screen.findByRole("heading", { level: 1, name: "テスト動画" });
      openTab("Work");
      openTab("People");
      await waitFor(() => expect(address()).toBe("/videos/7?tab=people"));

      fireEvent.click(screen.getByRole("link", { name: "Library" }));
      expect(await screen.findByText("ライブラリ")).toBeDefined();
      // One Back step returns to the video: tab changes replaced the entry instead of adding ones.
      fireEvent.click(screen.getByRole("button", { name: "Back" }));
      await screen.findByRole("heading", { level: 1, name: "テスト動画" });
      expect(selectedTab()).toBe("People2");
      expect(address()).toBe("/videos/7?tab=people");

      openTab("Overview");
      await waitFor(() => expect(address()).toBe("/videos/7"));
    });

    it("次の動画へ移ると選んだタブを保ち、アドレスにも書く", async () => {
      server.videos.set(8, { ...owned, id: 8, title: "後続の動画" });
      renderAt("/videos/7");
      await screen.findByRole("heading", { level: 1, name: "テスト動画" });
      openTab("File & media");
      fireEvent.click(screen.getByRole("link", { name: "Next video" }));
      await screen.findByRole("heading", { level: 1, name: "後続の動画" });
      expect(selectedTab()).toBe("File & media");
      await waitFor(() => expect(address()).toBe("/videos/8?tab=file"));
    });

    it("知らないタブ名や、この動画に無いタブを名指すアドレスでは Overview を開いてアドレスから外し、次の動画でもそのタブを開かない", async () => {
      const { unmount } = renderAt("/videos/7?tab=nope");
      await screen.findByRole("heading", { level: 1, name: "テスト動画" });
      expect(selectedTab()).toBe("Overview");
      await waitFor(() => expect(address()).toBe("/videos/7"));
      unmount();

      server.videos.set(8, { ...owned, id: 8, title: "後続の動画", workInfo: undefined });
      renderAt("/videos/8?tab=people");
      await screen.findByRole("heading", { level: 1, name: "後続の動画" });
      expect(tabNames()).not.toContain("People");
      expect(selectedTab()).toBe("Overview");
      await waitFor(() => expect(address()).toBe("/videos/8"));

      fireEvent.click(screen.getByRole("link", { name: "First video" }));
      await screen.findByRole("heading", { level: 1, name: "テスト動画" });
      expect(tabNames()).toContain("People2");
      expect(selectedTab()).toBe("Overview");
      expect(address()).toBe("/videos/7");
    });

    it("知らないタブ名を名指すリンクで移ると、前のタブではなく Overview を開いてアドレスから外す", async () => {
      server.videos.set(8, { ...owned, id: 8, title: "後続の動画" });
      renderAt("/videos/7?tab=people");
      await screen.findByRole("heading", { level: 1, name: "テスト動画" });
      expect(selectedTab()).toBe("People2");

      fireEvent.click(
        screen.getByRole("link", { name: "Next video with an unknown tab" }),
      );
      await screen.findByRole("heading", { level: 1, name: "後続の動画" });
      expect(selectedTab()).toBe("Overview");
      await waitFor(() => expect(address()).toBe("/videos/8"));
    });

    describe("作品情報プラグインの一覧を待つ Work", () => {
      it("開いたアドレスの Work は、承認済みのプラグインが届くと選ぶ", async () => {
        const answer = holdPlugins();
        server.videos.set(7, { ...owned, workInfo: undefined });
        renderAt("/videos/7?tab=work");
        await screen.findByRole("heading", { level: 1, name: "テスト動画" });
        expect(address()).toBe("/videos/7?tab=work");

        answer([approvedWorkPlugin]);
        await waitFor(() => expect(selectedTab()).toBe("Work"));
        expect(address()).toBe("/videos/7?tab=work");
      });

      it("開いたアドレスの Work は、承認済みのプラグインが無いと Overview にしてアドレスから外す", async () => {
        const answer = holdPlugins();
        server.videos.set(7, { ...owned, workInfo: undefined });
        renderAt("/videos/7?tab=work");
        await screen.findByRole("heading", { level: 1, name: "テスト動画" });

        answer([]);
        await waitFor(() => expect(address()).toBe("/videos/7"));
        expect(tabNames()).not.toContain("Work");
        expect(selectedTab()).toBe("Overview");
      });
    });
  });

  it("次の動画にもあるタブは移った後も選んだまま", async () => {
    server.videos.set(8, { ...owned, id: 8, title: "後続の動画" });
    await openPage();
    openTab("File & media");

    fireEvent.click(screen.getByRole("link", { name: "Next video" }));
    await screen.findByRole("heading", { level: 1, name: "後続の動画" });
    expect(selectedTab()).toBe("File & media");
  });

  it("承認済みの作品情報プラグインが届くと、選んだタブを変えずに空の作品へ Work を足す", async () => {
    const answer = holdPlugins();
    server.videos.set(7, { ...owned, workInfo: undefined });
    await openPage();
    openTab("File & media");
    expect(tabNames()).not.toContain("Work");

    answer([approvedWorkPlugin]);
    await waitFor(() => expect(tabNames()).toContain("Work"));
    expect(tabNames()).toEqual([
      "Overview",
      "Work",
      "Versions3",
      "Thumbnail",
      "File & media",
    ]);
    expect(selectedTab()).toBe("File & media");
  });

  describe("Versions", () => {
    const other: Video = { ...owned, id: 20, title: "別のバージョン" };

    function versionCalls(): string[] {
      return fetchMock.mock.calls
        .map(([input]) => String(input))
        .filter((url) => url.endsWith("/versions"));
    }

    it("一覧は Versions タブを選んでから読む", async () => {
      fetchMock.mockImplementation((input) => {
        const url = String(input);
        return Promise.resolve(
          url === "/api/videos/7/versions"
            ? json({ representativeId: 7, items: [owned, other] })
            : respond(url),
        );
      });
      await openPage();
      expect(versionCalls()).toEqual([]);

      openTab("Versions");
      expect(await screen.findByRole("list", { name: "Versions" })).toBeDefined();
      expect(versionCalls()).toEqual(["/api/videos/7/versions"]);
    });

    it("再生中のバージョンを自分の行から外すと、ページの動画が外した動画に替わり Overview を選ぶ（Edge Case）", async () => {
      const removed: Video = { ...owned, title: "外した動画", versions: undefined };
      fetchMock.mockImplementation((input, init) => {
        const url = String(input);
        if (url === "/api/videos/7/versions") {
          return Promise.resolve(json({ representativeId: 7, items: [owned, other] }));
        }
        if (url === "/api/videos/7/unbundle" && init?.method === "POST") {
          server.videos.set(7, removed);
          return Promise.resolve(json(removed));
        }
        return Promise.resolve(respond(url));
      });
      await openPage();
      openTab("Versions");
      const list = await screen.findByRole("list", { name: "Versions" });
      const current = within(list).getAllByRole("listitem")[0] as HTMLElement;
      expect(current.getAttribute("aria-current")).toBe("true");
      fireEvent.click(
        within(current).getByRole("button", { name: "Remove from versions" }),
      );

      await screen.findByRole("heading", { level: 1, name: "外した動画" });
      expect(tabNames().some((name) => name.startsWith("Versions"))).toBe(false);
      expect(selectedTab()).toBe("Overview");
      await waitFor(() =>
        expect(document.activeElement).toBe(
          screen.getByRole("tab", { name: "Overview" }),
        ),
      );
    });
  });
});
