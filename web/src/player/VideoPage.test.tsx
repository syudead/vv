import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { Link, MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AuthState } from "../api/auth";
import {
  type RelatedVideos,
  setRenderedAudience,
  type SubtitleTrack,
  type Video,
} from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { updateFavorites } from "../api/favorites";
import type { GroupItem } from "../api/groups";
import { saveListSnapshot, takeListSnapshot } from "../api/listSnapshot";
import { type Audience, AudienceProvider } from "../auth/audience";
import { reloadPage } from "../auth/pageNavigation";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { formatBytes, formatDuration } from "../lib/format";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import type { PlaybackFailureKind } from "./playbackRecovery";
import type { PlayerControls } from "./playerControls";
import { technicalSummary } from "./properties";
import type { PlayerStatus } from "./VideoPlayer";
import VideoPage from "./VideoPage";

interface PlayerProps {
  video: Video;
  initialPositionMs: number;
  autoplay: boolean;
  onPosition: (positionMs: number) => void;
  onProgress: (positionMs: number, immediate: boolean) => void;
  onPlay?: () => void;
  onError: (positionMs: number, kind: PlaybackFailureKind) => void;
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
  subtitles?: readonly SubtitleTrack[];
}

vi.mock("../auth/pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../auth/pageNavigation")>()),
  reloadPage: vi.fn(),
}));

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerProps | undefined,
  mounts: 0,
  controls: undefined as PlayerControls | undefined,
}));

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: PlayerProps) {
    playerMock.props = props;
    // The player reads these values only when created.
    const [initial] = useState(props);
    useEffect(() => {
      playerMock.mounts += 1;
      if (playerMock.controls !== undefined) initial.onControls(playerMock.controls);
      return () => initial.onControls(null);
    }, [initial]);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: (video: Video) =>
    video.probeState === "done" &&
    (video.durationMs ?? 0) > 0 &&
    video.videoCodec !== undefined,
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

const video: Video = {
  id: 7,
  virtual: false,
  title: "テスト動画",
  public: false,
  sizeBytes: 84_331_821,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  fileModifiedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  thumbnailUrl: "/api/videos/7/thumbnail",
  previewState: "done",
  seekThumbnailState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  location: { path: "/media/movies/テスト動画.mp4", openable: true },
  tags: [],
};

function related(id: number, title: string): Video {
  return { ...video, id, title, location: undefined, thumbnailUrl: undefined };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const server = {
  videos: new Map<number, (() => Response) | Video[]>(),
  related: new Map<number, RelatedVideos>(),
  subtitles: new Map<number, () => Response>(),
  probe: vi.fn<() => Response>(),
  open: vi.fn<() => Response>(),
  grouping:
    vi.fn<(method: string, url: string, body: unknown) => Response | Promise<Response>>(),
  groupItems: new Map<number, GroupItem[]>(),
  thumbnailPosition: vi.fn<(body: unknown) => Response | Promise<Response>>(),
  versions: vi.fn<(method: string, id: number, suffix: string) => Response>(),
  session: "owner" as AuthState,
};

function fakeControls(): PlayerControls {
  return {
    togglePlay: vi.fn(),
    play: vi.fn(),
    pause: vi.fn(),
    seekTo: vi.fn(),
    restart: vi.fn(),
    toggleMute: vi.fn(),
    toggleFullscreen: vi.fn(),
    toggleSubtitles: vi.fn(),
    isFullscreen: vi.fn(() => false),
    menuOpen: vi.fn(() => false),
    wake: vi.fn(),
    positionMs: vi.fn(() => 0),
  };
}

function Screen({ name }: { name: string }) {
  const location = useLocation();
  return (
    <p data-testid="screen">
      {name} {location.pathname}
      {location.search}
    </p>
  );
}

function renderPage(
  id = "7",
  from?: string,
  audience: Audience = "owner",
  autoplay = false,
  trail?: number[],
) {
  let state: Record<string, unknown> | undefined =
    from === undefined ? undefined : autoplay ? { from, autoplay } : { from };
  if (trail !== undefined) state = { ...state, trail };
  // IDs may include a query, such as 12?group=1.
  const [pathId, search] = id.split("?");
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter
          initialEntries={[
            {
              pathname: `/videos/${pathId ?? ""}`,
              search: search === undefined ? "" : `?${search}`,
              state,
            },
          ]}
        >
          <AudienceProvider audience={audience}>
            <Link to="/videos/8">別の動画</Link>
            <Link to="/videos/invalid">無効な動画</Link>
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
              <Route path="/" element={<Screen name="ライブラリ" />} />
              <Route path="/folders/*" element={<Screen name="フォルダ" />} />
              <Route path="/groups/:id" element={<Screen name="グループ" />} />
              <Route
                path="/smart-folders/:id"
                element={<Screen name="スマートフォルダ" />}
              />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

async function ready() {
  return screen.findByRole("heading", { level: 1 });
}

function closeButton() {
  return screen.getByRole("button", { name: "Close" });
}

function player(): PlayerProps {
  const props = playerMock.props;
  if (props === undefined) throw new Error("player が描画されていません");
  return props;
}

/** Radix tabs select on mouse down, not on click. */
function openTab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name: new RegExp(`^${name}`) }));
}

describe("VideoPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    playerMock.props = undefined;
    playerMock.mounts = 0;
    playerMock.controls = fakeControls();
    server.videos.clear();
    server.related.clear();
    server.subtitles.clear();
    server.videos.set(7, [video]);
    server.related.set(7, {
      items: [related(8, "後続の動画"), related(3, "前の動画")],
      nextId: 8,
      prevId: 3,
    });
    server.probe.mockReset();
    server.open.mockReset();
    server.grouping.mockReset();
    server.groupItems.clear();
    server.thumbnailPosition.mockReset();
    server.versions.mockReset();
    fetchMock.mockReset();
    installFakeEventSource();
    server.session = "owner";
    vi.mocked(reloadPage).mockClear();
    setRenderedAudience(null);
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/auth/session")
        return Promise.resolve(json({ state: server.session }));
      const items = /^\/api\/groups\/(\d+)\/items\?limit=200$/.exec(url);
      if (items !== null && method === "GET") {
        const found = server.groupItems.get(Number(items[1]));
        return Promise.resolve(
          found === undefined
            ? json({ code: "not_found", message: "x" }, 404)
            : json({ items: found }),
        );
      }
      if (url.startsWith("/api/folders/") || url.startsWith("/api/groups/")) {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
        return Promise.resolve(server.grouping(method, url, body));
      }
      if (url === "/api/tags") return Promise.resolve(json([]));
      // Group/smart-folder-only queries use the same response as the bare URL.
      const match =
        /^\/api\/videos\/(\d+)(\/[a-z-]+)?(?:\?(?:group|smartFolder)=\d+)?$/.exec(url);
      if (match === null) return Promise.resolve(json({}));
      const id = Number(match[1]);
      const suffix = match[2];
      if (suffix === "/subtitles") {
        const answer = server.subtitles.get(id);
        return Promise.resolve(answer?.() ?? json({ subtitles: [] }));
      }
      if (suffix === "/related") {
        return Promise.resolve(json(server.related.get(id) ?? { items: [] }));
      }
      if (suffix === "/probe" && method === "POST")
        return Promise.resolve(server.probe());
      if (suffix === "/open" && method === "POST") return Promise.resolve(server.open());
      if (suffix === "/progress") return Promise.resolve(json({}));
      if (
        suffix === "/versions" ||
        suffix === "/make-representative" ||
        suffix === "/unbundle"
      ) {
        return Promise.resolve(server.versions(method, id, suffix));
      }
      if (suffix === "/thumbnail-position" && method === "PUT") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
        return Promise.resolve(server.thumbnailPosition(body));
      }
      const entry = server.videos.get(id);
      if (entry === undefined) {
        return Promise.resolve(
          json({ code: "not_found", message: "見つかりません" }, 404),
        );
      }
      if (typeof entry === "function") return Promise.resolve(entry());
      // Consume response arrays in order, then repeat the last response.
      const next = entry.length > 1 ? entry.shift() : entry[0];
      return Promise.resolve(json(next));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  describe("画面の構成", () => {
    it("題名・タブ・ファイルの情報・技術情報・関連動画を出し、廃止した項目を出さない", async () => {
      renderPage("7", "/?q=abc");
      expect((await ready()).textContent).toBe("テスト動画");
      // The title updates in an effect after the heading first renders.
      await waitFor(() => expect(document.title).toBe("テスト動画 · VVMDM"));
      expect(screen.getByRole("tab", { name: "Overview", selected: true })).toBeDefined();
      openTab("File & media");
      expect(screen.getByText(/H\.264/)).toBeDefined();
      expect(screen.getByRole("button", { name: "Open file" })).toBeDefined();
      expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
      expect(
        await screen.findByRole("heading", { level: 2, name: "Related videos" }),
      ).toBeDefined();

      expect(screen.queryByRole("link", { name: /ライブラリ|フォルダ/ })).toBeNull();
      const text = document.body.textContent ?? "";
      for (const removed of [
        "Watched",
        "Resume from",
        "Analysis",
        "Browser playback",
        "bytes",
        "1080p",
      ]) {
        expect(text).not.toContain(removed);
      }
      expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);
    });

    it("× で遷移元の一覧へ、無ければ / へ戻る", async () => {
      renderPage("7", "/folders/3/A%20B?sort=titleAsc");
      await ready();
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe(
        "フォルダ /folders/3/A%20B?sort=titleAsc",
      );
    });

    it("Esc で遷移元の一覧へ戻る", async () => {
      renderPage("7", "/?q=abc");
      await ready();
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=abc");
    });

    it("全画面中の Esc では閉じない", async () => {
      const controls = fakeControls();
      vi.mocked(controls.isFullscreen).mockReturnValue(true);
      playerMock.controls = controls;
      renderPage("7", "/?q=abc");
      await ready();
      await screen.findByTestId("video-player");
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(screen.queryByTestId("screen")).toBeNull();
      expect(controls.isFullscreen).toHaveBeenCalled();
    });

    it("開けない環境では「ファイルを開く」を出さない", async () => {
      server.videos.set(7, [
        { ...video, location: { path: "/media/a.mp4", openable: false } },
      ]);
      renderPage();
      await ready();
      openTab("File & media");
      expect(screen.queryByRole("button", { name: "Open file" })).toBeNull();
      expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
    });

    it("見出しの帯に置き場所のパンくずを出し、段からそのフォルダ画面へ移る", async () => {
      server.videos.set(7, [
        { ...video, folder: { rootId: 1, path: "movies", rootName: "media" } },
      ]);
      renderPage("7", "/?q=abc");
      await ready();
      const nav = screen.getByRole("navigation", { name: "Folder" });
      fireEvent.click(within(nav).getByRole("link", { name: "movies" }));
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
    });

    it("ロゴからホームへ移る", async () => {
      renderPage("7", "/folders/1/movies");
      await ready();
      fireEvent.click(screen.getByRole("link", { name: "VVMDM home" }));
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /");
    });

    it("開く要求が file_missing ならパネルの末尾に出し、画面の他の部分は変えない", async () => {
      server.open.mockReturnValue(json({ code: "file_missing", message: "無い" }, 409));
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      openTab("File & media");
      const panel = screen.getByRole("tabpanel");
      const snapshot = () => ({
        header: document.querySelector("header")?.outerHTML,
        frame: document.querySelector("[data-player-frame]")?.outerHTML,
        title: document.querySelector("h1")?.outerHTML,
        media: within(panel).getByRole("region", { name: "Media" }).outerHTML,
        related: document.querySelector("aside")?.outerHTML,
      });
      const before = snapshot();
      fireEvent.click(screen.getByRole("button", { name: "Open file" }));
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe(
        "Couldn't open the file: The video file is missing.",
      );
      expect(screen.getAllByRole("alert")).toHaveLength(1);
      expect(screen.getByTestId("video-player")).toBeDefined();
      expect(panel.firstElementChild?.lastElementChild).toBe(alert);
      expect(snapshot()).toEqual(before);
    });
  });

  describe("位置と大きさ", () => {
    it("開いたときと別の動画へ移ったときに、ページの先頭へ戻す", async () => {
      const scrollTo = vi.fn();
      vi.stubGlobal("scrollTo", scrollTo);
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      renderPage("7", "/?q=a");
      await ready();
      expect(scrollTo).toHaveBeenCalledWith(0, 0);
      scrollTo.mockClear();
      fireEvent.click(await screen.findByRole("link", { name: /後続の動画/ }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画"),
      );
      expect(scrollTo).toHaveBeenCalledWith(0, 0);
    });

    it("プレイヤーの入れ物は列を 1 本（grid-cols-1、minmax(0,1fr)）に固定し、層を幅の中で折り返させる", async () => {
      renderPage();
      await ready();
      // Without a column constraint, overlay content can overflow narrow screens.
      const frame = document.querySelector("[data-player-frame]");
      expect(frame?.className.split(" ")).toContain("grid-cols-1");
    });
  });

  describe("字幕", () => {
    const ja: SubtitleTrack = { file: "テスト動画.ja.srt", label: "ja", format: "srt" };
    const en: SubtitleTrack = { file: "後続.en.vtt", label: "en", format: "vtt" };

    it("開いた動画の字幕の一覧をプレイヤーに渡し、別の動画へ移ったら前の一覧を渡さない", async () => {
      server.subtitles.set(7, () => json({ subtitles: [ja] }));
      server.subtitles.set(8, () => json({ subtitles: [en] }));
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      renderPage();
      await ready();
      await waitFor(() => expect(player().subtitles).toEqual([ja]));
      const seen: (readonly SubtitleTrack[] | undefined)[] = [];
      fireEvent.click(await screen.findByRole("link", { name: /後続の動画/ }));
      await waitFor(() => {
        seen.push(player().subtitles);
        expect(player().video.id).toBe(8);
        expect(player().subtitles).toEqual([en]);
      });
      expect(seen.some((list) => list?.includes(ja) === true)).toBe(false);
    });

    it("一覧を取れなければ字幕無しで再生を続ける", async () => {
      server.subtitles.set(7, () =>
        json({ code: "file_unavailable", message: "読めません" }, 404),
      );
      renderPage();
      await ready();
      await waitFor(() =>
        expect(
          fetchMock.mock.calls.some(
            ([input]) => String(input) === "/api/videos/7/subtitles",
          ),
        ).toBe(true),
      );
      await act(async () => undefined);
      expect(player().subtitles).toEqual([]);
      expect(screen.getByTestId("video-player")).toBeDefined();
      expect(screen.queryByRole("alert")).toBeNull();
    });
  });

  describe("Related videos", () => {
    it("関連動画から移ったあとの × は最初の一覧へ戻る", async () => {
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      renderPage("7", "/folders/1/movies");
      await ready();
      fireEvent.click(await screen.findByRole("link", { name: /後続の動画/ }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画"),
      );
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
    });

    it("0 件のとき見出しを出さず、× は残す", async () => {
      server.related.set(7, { items: [] });
      renderPage();
      await ready();
      await waitFor(() => expect(screen.queryByText("Related videos")).toBeNull());
      await act(async () => undefined);
      expect(
        screen.queryByRole("heading", { level: 2, name: "Related videos" }),
      ).toBeNull();
      expect(closeButton()).toBeDefined();
    });
  });

  describe("プレイヤーの操作", () => {
    it("› で関連動画の先頭へ移り、そこで ‹ を押すと元の動画へ戻る（受け入れ条件 4）", async () => {
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      server.related.set(8, { items: [related(3, "別の動画")], nextId: 3 });
      renderPage("7", "/folders/1/movies");
      await ready();
      const next = await screen.findByRole("button", { name: "Next video: 後続の動画" });
      expect(screen.queryByRole("button", { name: /^Previous video/ })).toBeNull();
      fireEvent.click(next);
      await waitFor(() => expect(player().video.id).toBe(8));
      expect(player().autoplay).toBe(false);

      fireEvent.click(
        await screen.findByRole("button", { name: "Previous video: テスト動画" }),
      );
      await waitFor(() => expect(player().video.id).toBe(7));
      await screen.findByRole("button", { name: "Next video: 後続の動画" });
      expect(screen.queryByRole("button", { name: /^Previous video/ })).toBeNull();
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
    });

    it("関連動画の行から移った先でも、‹ で元の動画へ戻れる", async () => {
      server.videos.set(3, [{ ...related(3, "前の動画"), location: video.location }]);
      server.related.set(3, { items: [related(8, "後続の動画")], nextId: 8 });
      renderPage();
      await ready();
      fireEvent.click(await screen.findByRole("link", { name: /^前の動画/ }));
      await waitFor(() => expect(player().video.id).toBe(3));
      fireEvent.click(
        await screen.findByRole("button", { name: "Previous video: テスト動画" }),
      );
      await waitFor(() => expect(player().video.id).toBe(7));
    });

    it("再生中に「次の動画」で移ると、移った先でも再生を続ける", async () => {
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      renderPage();
      await ready();
      const next = await screen.findByRole("button", { name: "Next video: 後続の動画" });
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      fireEvent.click(next);
      await waitFor(() => expect(player().video.id).toBe(8));
      expect(player().autoplay).toBe(true);
    });

    it("履歴の「Resume」から来たら、再開の規則の位置から自動で再生を始める（043 R-13）", async () => {
      server.videos.set(7, [
        {
          ...video,
          progress: {
            positionMs: 65_000,
            completed: false,
            updatedAt: "2026-09-02T00:00:00Z",
          },
        },
      ]);
      renderPage("7", "/history?watch=inProgress", "owner", true);
      await ready();
      expect(player().autoplay).toBe(true);
      expect(player().initialPositionMs).toBe(65_000);
    });

    it("履歴の「Start over」から来た見終わった動画は、先頭から自動で再生を始める（043 R-13）", async () => {
      server.videos.set(7, [
        {
          ...video,
          progress: {
            positionMs: 240_000,
            completed: true,
            updatedAt: "2026-09-02T00:00:00Z",
          },
        },
      ]);
      renderPage("7", "/history", "owner", true);
      await ready();
      expect(player().autoplay).toBe(true);
      expect(player().initialPositionMs).toBe(0);
    });

    it("アドレスの t があれば、保存した位置ではなくその秒から自動で再生を始める", async () => {
      server.videos.set(7, [
        {
          ...video,
          progress: {
            positionMs: 65_000,
            completed: false,
            updatedAt: "2026-09-02T00:00:00Z",
          },
        },
      ]);
      renderPage("7?t=130", "/people/3", "owner", true);
      await ready();
      expect(player().autoplay).toBe(true);
      expect(player().initialPositionMs).toBe(130_000);
    });

    it("前後の矢印は操作バーと同じ時期に見せ、前後が無い側は出さない", async () => {
      server.videos.set(3, [{ ...related(3, "前の動画"), location: video.location }]);
      server.related.set(7, { items: [] });
      renderPage("7", undefined, "owner", false, [3]);
      await ready();
      const previous = await screen.findByRole("button", {
        name: "Previous video: 前の動画",
      });
      expect(screen.queryByRole("button", { name: /^Next video/ })).toBeNull();
      expect(previous.className).toContain("opacity-100");
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: false,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      expect(previous.className).toContain("opacity-0");
      expect(previous.className).toContain("pointer-events-none");
    });

    it("全画面の間は、前後の矢印の題名の吹き出しを全画面の入れ物の中に描く", async () => {
      renderPage();
      await ready();
      const next = await screen.findByRole("button", { name: "Next video: 後続の動画" });
      const frame = document.querySelector<HTMLElement>("[data-player-frame]");
      Object.defineProperty(document, "fullscreenElement", {
        configurable: true,
        value: frame,
      });
      try {
        act(() => {
          document.dispatchEvent(new Event("fullscreenchange"));
        });
        act(() => next.focus());
        const tip = await screen.findByRole("tooltip");
        expect(frame?.contains(tip)).toBe(true);
      } finally {
        Object.defineProperty(document, "fullscreenElement", {
          configurable: true,
          value: null,
        });
      }
    });

    it("グループに属さない動画は、応答の prevId ではなく直前に見ていた動画を ‹ にする", async () => {
      server.related.set(7, { items: [related(8, "後続の動画")], nextId: 8, prevId: 99 });
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "Next video: 後続の動画" });
      expect(screen.queryByRole("button", { name: /^Previous video/ })).toBeNull();
    });

    it("直前に見ていた動画が削除済みなら ‹ を出さない", async () => {
      renderPage("7", undefined, "owner", false, [99]);
      await ready();
      await screen.findByRole("button", { name: "Next video: 後続の動画" });
      await act(async () => undefined);
      expect(screen.queryByRole("button", { name: /^Previous video/ })).toBeNull();
    });

    it("関連動画が無いときは › を出さない", async () => {
      server.related.set(7, { items: [] });
      renderPage();
      await ready();
      await act(async () => undefined);
      expect(screen.queryByRole("button", { name: /^Next video/ })).toBeNull();
    });

    it("画面のどこでも Space・0 がプレイヤーに効く", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      await screen.findByTestId("video-player");
      // Wait for page shortcuts to receive the player controls before testing Space.
      await waitFor(() => {
        fireEvent.keyDown(document.body, { key: "0" });
        expect(controls.seekTo).toHaveBeenCalledWith(0);
      });
      fireEvent.keyDown(document.body, { key: " " });
      expect(controls.togglePlay).toHaveBeenCalledTimes(1);
    });
  });

  describe("プレイヤーの中の状態", () => {
    const pending: Video = {
      ...video,
      probeState: "pending",
      thumbnailState: "pending",
      previewState: "pending",
      seekThumbnailState: undefined,
      durationMs: undefined,
      width: undefined,
      height: undefined,
      container: undefined,
      videoCodec: undefined,
      audioCodec: undefined,
      thumbnailUrl: undefined,
    };

    async function advance(ms: number) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(ms);
      });
    }

    it("読み取り前は段階表示を出し、pending から done で再生できるようになる", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      server.videos.set(7, [
        pending,
        {
          ...video,
          thumbnailState: "pending",
          previewState: "pending",
          seekThumbnailState: "pending",
        },
        { ...video, previewState: "pending" },
        { ...video, previewState: "failed" },
      ]);
      renderPage();
      await ready();
      const stages = await screen.findByRole("status", { name: "" });
      expect(
        within(stages)
          .getAllByRole("listitem")
          .map((row) => row.textContent),
      ).toEqual([
        "Finding the fileDone",
        "Reading video informationIn progress",
        "ThumbnailWaiting",
        "Seek previewWaiting",
        "List previewWaiting",
      ]);
      expect(screen.getByText("Getting ready to play")).toBeDefined();
      expect(screen.queryByTestId("video-player")).toBeNull();

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(screen.getByTestId("video-player")).toBeDefined();
      expect(screen.queryByText("Getting ready to play")).toBeNull();
      expect(
        screen.getByText(
          "Creating the thumbnail, the seek preview, and the list preview · You can play the video now",
        ),
      ).toBeDefined();

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(
        screen.getByText("Creating the list preview · You can play the video now"),
      ).toBeDefined();
      expect(playerMock.mounts).toBe(1);

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(screen.queryByText(/Creating/)).toBeNull();
      const calls = fetchMock.mock.calls.filter(
        ([input]) => String(input) === "/api/videos/7",
      );
      expect(calls).toHaveLength(4);
      await advance(10_000);
      expect(
        fetchMock.mock.calls.filter(([input]) => String(input) === "/api/videos/7"),
      ).toHaveLength(4);
      expect(playerMock.mounts).toBe(1);
    });

    it("取り直しで読み取りが失敗したら、段階表示から読み取り失敗の表示へ切り替わる", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      server.videos.set(7, [
        pending,
        {
          ...pending,
          probeState: "failed",
          probeError: "moov atom not found",
          probeErrorCode: "probe_failed",
        },
      ]);
      renderPage();
      await screen.findByText("Getting ready to play");
      await emitServerEvent("video", { id: 7 });
      await advance(0);
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("Couldn't read this video")).toBeDefined();
      expect(
        within(alert).getByText("The file is damaged or isn't a supported video."),
      ).toBeDefined();
      expect(within(alert).queryByText(/moov atom/)).toBeNull();
      expect(within(alert).getByRole("button", { name: "Open file" })).toBeDefined();
      await advance(10_000);
      expect(
        fetchMock.mock.calls.filter(([input]) => String(input) === "/api/videos/7"),
      ).toHaveLength(2);
      expect(screen.queryByText(/Creating/)).toBeNull();
    });

    it("「もう一度読み取る」の 409 でも取り直して段階表示へ移る", async () => {
      server.videos.set(7, [
        { ...pending, probeState: "failed", probeError: "壊れています" },
        pending,
      ]);
      server.probe.mockReturnValue(
        json({ code: "probe_not_failed", message: "読み取り中です" }, 409),
      );
      renderPage();
      fireEvent.click(await screen.findByRole("button", { name: "Read again" }));
      expect(await screen.findByText("Getting ready to play")).toBeDefined();
      expect(server.probe).toHaveBeenCalledTimes(1);
      expect(screen.queryByText("Couldn't start reading the video again")).toBeNull();
    });

    it("「もう一度読み取る」が受け付けられなければ、その旨を出してボタンを戻す", async () => {
      server.videos.set(7, [
        { ...pending, probeState: "failed", probeError: "壊れています" },
      ]);
      server.probe.mockReturnValue(json({ code: "internal", message: "失敗" }, 500));
      renderPage();
      const button = await screen.findByRole("button", { name: "Read again" });
      fireEvent.click(button);
      expect(
        await screen.findByText("Couldn't start reading the video again"),
      ).toBeDefined();
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });

    it("取り直しが 404 なら、プレイヤーの中に「開けません」を出し、× は残す", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      let calls = 0;
      server.videos.set(7, () => {
        calls += 1;
        return calls === 1
          ? json({ ...video, previewState: "pending" })
          : json({ code: "not_found", message: "見つかりません" }, 404);
      });
      renderPage("7", "/?q=a");
      await ready();
      await emitServerEvent("video", { id: 7 });
      await advance(0);
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("This video can't be opened")).toBeDefined();
      expect(screen.queryByTestId("video-player")).toBeNull();
      expect(closeButton()).toBeDefined();
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=a");
    });

    it("最初の取得が 404 でも「開けません」を出し、再試行は置かない", async () => {
      server.videos.delete(7);
      renderPage();
      expect(await screen.findByText("This video can't be opened")).toBeDefined();
      expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    });

    it("最初の取得が 404 以外で失敗したら理由と「再試行」を出し、取り直せる", async () => {
      let calls = 0;
      server.videos.set(7, () => {
        calls += 1;
        return calls === 1
          ? json({ code: "internal", message: "データベースに届きません" }, 500)
          : json(video);
      });
      renderPage();
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("Couldn't load this video")).toBeDefined();
      expect(
        within(alert).getByText("Something went wrong on the server."),
      ).toBeDefined();
      expect(screen.queryByText("This video can't be opened")).toBeNull();
      fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
      expect((await ready()).textContent).toBe("テスト動画");
      expect(screen.getByTestId("video-player")).toBeDefined();
    });

    it("再生失敗の再試行は失敗した位置から、自動で再生を始める", async () => {
      renderPage();
      await ready();
      act(() => player().onError(42_000, "source"));
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("Couldn't play this video")).toBeDefined();
      fireEvent.click(within(alert).getByRole("button", { name: "Try again from 0:42" }));
      await waitFor(() => expect(playerMock.mounts).toBe(2));
      expect(player().initialPositionMs).toBe(42_000);
      expect(player().autoplay).toBe(true);
      expect(screen.queryByText("Couldn't play this video")).toBeNull();
    });

    it("通信の失敗は、サーバーに届かないことを伝えて失敗した位置から再試行できる", async () => {
      renderPage();
      await ready();
      act(() => player().onError(42_000, "network"));
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("Couldn't reach the server")).toBeDefined();
      expect(within(alert).queryByText(/moved or deleted/)).toBeNull();
      expect(
        within(alert).getByRole("button", { name: "Try again from 0:42" }),
      ).toBeDefined();
    });

    it("データが読めない失敗は、ファイルが壊れているかもしれないと伝える", async () => {
      renderPage();
      await ready();
      act(() => player().onError(1000, "decode"));
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText(/The file may be damaged/)).toBeDefined();
    });

    it("読み込み直しの間は、失敗の層ではなく再接続中の表示を出す", async () => {
      renderPage();
      await ready();
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: true,
          playing: false,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      expect(screen.getByText("Connection lost · Reconnecting")).toBeDefined();
      expect(screen.queryByRole("alert")).toBeNull();
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      expect(screen.queryByText("Connection lost · Reconnecting")).toBeNull();
    });

    it("映像の読み込み中は文字を伴う状態を表示し、再生開始後に消す", async () => {
      renderPage();
      await ready();
      act(() =>
        player().onStatus({
          loading: true,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      const status = screen.getByRole("status");
      expect(status.textContent).toBe("Loading");
      expect(status.className).toContain("bg-navbar");
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      expect(screen.queryByRole("status")).toBeNull();
    });

    it("途中まで見た動画は続きの位置から始め、再開を知らせる表示を出さない", async () => {
      server.videos.set(7, [
        {
          ...video,
          progress: {
            positionMs: 65_000,
            completed: false,
            updatedAt: "2026-09-02T00:00:00Z",
          },
        },
      ]);
      renderPage();
      await ready();
      expect(player().initialPositionMs).toBe(65_000);
      expect(player().autoplay).toBe(false);
      expect(document.body.textContent).not.toContain("から再開");
    });
  });

  describe("再生終了", () => {
    function end() {
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: false,
          userActive: true,
          ended: true,
          stalled: false,
          positioned: true,
        }),
      );
    }

    it("ended で次の動画と「次を再生」「もう一度見る」を出し、もう一度見るで先頭から再生する", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      end();
      expect(screen.getByRole("status").textContent).toBe("Playback finished");
      expect(
        screen.getByRole("heading", { level: 2, name: "Playback finished" }),
      ).toBeDefined();
      expect(screen.getByText("Next video")).toBeDefined();
      expect(screen.getAllByRole("link", { name: /後続の動画/ })).toHaveLength(2);
      fireEvent.click(screen.getByRole("button", { name: "Watch again" }));
      expect(controls.restart).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("button", { name: "Play next" })).toBeDefined();
    });

    it("フォーカスがプレイヤーの中にあったときだけ、主な操作へフォーカスを移す", async () => {
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      (await screen.findByRole("button", { name: /^Next video/ })).focus();
      end();
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Play next" }),
      );
    });

    it("プレイヤーの外（関連動画）にいる人のフォーカスは奪わない", async () => {
      renderPage();
      await ready();
      const link = await screen.findByRole("link", { name: /後続の動画/ });
      link.focus();
      end();
      expect(screen.getByRole("button", { name: "Play next" })).toBeDefined();
      expect(document.activeElement).toBe(link);
    });

    it("同じフォルダの後続が無ければ「次を再生」を出さない", async () => {
      server.related.set(7, { items: [related(3, "前の動画")] });
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      end();
      expect(screen.getByRole("status").textContent).toBe("Playback finished");
      expect(
        screen.getByRole("heading", { level: 2, name: "Playback finished" }),
      ).toBeDefined();
      expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "Play next" })).toBeNull();
    });

    it("「次を再生」で戻り先付きで次の動画へ移り、移った先で再生を始める", async () => {
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      renderPage("7", "/folders/1/movies");
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      end();
      fireEvent.click(screen.getByRole("button", { name: "Play next" }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画"),
      );
      await waitFor(() => expect(player().video.id).toBe(8));
      expect(player().autoplay).toBe(true);
      expect(screen.queryByText("Playback finished")).toBeNull();
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
    });
  });

  describe("グループのメンバー", () => {
    const folder = { rootId: 1, path: "series" };
    const member = (id: number, title: string, position: number): Video => ({
      ...video,
      id,
      title,
      group: { id: 1, folder, name: "series", position, count: 3 },
    });
    const ep01 = member(11, "ep01", 1);
    const ep02 = member(12, "ep02", 2);
    const ep03 = member(13, "ep03", 3);
    const listed = (value: Video): Video => ({ ...value, group: undefined });
    const group = {
      id: 1,
      name: "series",
      items: [ep01, ep02, ep03].map(listed),
      offset: 0,
      total: 3,
    };

    beforeEach(() => {
      server.videos.set(11, [ep01]);
      server.videos.set(12, [ep02]);
      server.videos.set(13, [ep03]);
      server.related.set(11, { items: [related(8, "後続の動画")], nextId: 12, group });
      server.related.set(12, {
        items: [related(8, "後続の動画")],
        nextId: 13,
        prevId: 11,
        group,
      });
      server.related.set(13, { items: [related(8, "後続の動画")], prevId: 12, group });
    });

    function end() {
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: false,
          userActive: true,
          ended: true,
          stalled: false,
          positioned: true,
        }),
      );
    }

    async function advance(ms: number) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(ms);
      });
    }

    async function openMember(id = "12") {
      renderPage(id, "/?q=series");
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Up next" });
    }

    const announcement = 'Playback finished. The next video, "ep03", plays in 5 seconds';

    function videoRequests(id: number) {
      return fetchMock.mock.calls.filter(
        ([input, init]) =>
          String(input) === `/api/videos/${String(id)}` &&
          (init?.method ?? "GET") === "GET",
      ).length;
    }

    const lineLabel = 'Group "series", video 2 of 3. Open the group';
    const groupLine = () => screen.findByRole("link", { name: lineLabel });
    const actionsButton = () => screen.findByRole("button", { name: "Group actions" });

    it("題名の上にグループ名と何本目かの1行を出し、グループのページへのリンクにする", async () => {
      await openMember();
      const link = await groupLine();
      expect(link.textContent).toBe("series·2 / 3");
      expect(link.getAttribute("href")).toBe("/groups/1");
      expect(screen.getByTitle("series")).toBeDefined();
      expect(link.nextElementSibling).toBe(await actionsButton());
      fireEvent.click(link);
      expect(screen.getByTestId("screen").textContent).toBe("グループ /groups/1");
    });

    it("カードもメンバーの一覧もドラッグの層に加えず、メンバーを押して動かしても何も持ち上げない（050 受け入れ条件 9）", async () => {
      await openMember();
      expect(document.querySelector("[data-drag-card]")).toBeNull();
      const member = screen.getByRole("link", { name: /^ep03\b/ });
      const pointer = { pointerType: "mouse", isPrimary: true, button: 0, pointerId: 1 };
      fireEvent.pointerDown(member, { ...pointer, clientX: 10, clientY: 10 });
      fireEvent.pointerMove(document, { ...pointer, clientX: 50, clientY: 50 });
      expect(document.querySelector("[data-drag-preview]")).toBeNull();
      fireEvent.pointerUp(document, { ...pointer, clientX: 50, clientY: 50 });
      expect(
        fetchMock.mock.calls.some(([input]) =>
          String(input).startsWith("/api/organize/"),
        ),
      ).toBe(false);
    });

    it("手で作ったグループ（folder が無い）も同じリンクと操作のボタンにする", async () => {
      server.videos.set(12, [
        { ...ep02, group: { id: 9, name: "picks", position: 2, count: 3 } },
      ]);
      await openMember();
      const link = await screen.findByRole("link", {
        name: 'Group "picks", video 2 of 3. Open the group',
      });
      expect(link.getAttribute("href")).toBe("/groups/9");
      expect(await actionsButton()).toBeDefined();
    });

    it("ゲストにも同じリンクを出し、グループ操作のボタンとグループの項目の読み取りは無い", async () => {
      renderPage("12", "/", "guest");
      const link = await groupLine();
      expect(link.getAttribute("href")).toBe("/groups/1");
      expect(screen.queryByRole("button", { name: "Group actions" })).toBeNull();
      expect(
        fetchMock.mock.calls.some(([input]) => String(input).startsWith("/api/groups/")),
      ).toBe(false);
    });

    describe("グループ操作", () => {
      const single = (value: Video): Video => ({ ...value, group: undefined });
      const menuTexts = async () =>
        within(await screen.findByRole("menu"))
          .getAllByRole("menuitem")
          .map((item) => item.textContent);

      function leaveOnServer() {
        server.videos.set(12, [single(ep02)]);
        server.related.set(12, { items: [related(8, "後続の動画")], nextId: 8 });
      }

      async function expectPlainVideo() {
        await waitFor(() => {
          const heading = screen.getByRole("heading", { level: 1 });
          expect(heading.previousElementSibling).toBeNull();
        });
        await waitFor(() =>
          expect(screen.queryByRole("heading", { level: 2, name: "Up next" })).toBeNull(),
        );
      }

      it("直接の項目なら「Remove from this group」を先頭に、区切りの後に「Dissolve…」を出す", async () => {
        const user = userEvent.setup();
        server.groupItems.set(1, [{ id: 52, kind: "video", video: listed(ep02) }]);
        await openMember();
        await waitFor(() =>
          expect(
            fetchMock.mock.calls.some(
              ([input]) => String(input) === "/api/groups/1/items?limit=200",
            ),
          ).toBe(true),
        );
        await user.click(await actionsButton());
        await waitFor(async () =>
          expect(await menuTexts()).toEqual([
            "Remove from this group",
            "Turn the group into a tag…",
            "Dissolve…",
          ]),
        );
        expect(screen.getByRole("separator")).toBeDefined();
      });

      it("子グループを通してだけ入っている動画には「Remove from this group」を出さない", async () => {
        const user = userEvent.setup();
        server.groupItems.set(1, [
          { id: 51, kind: "video", video: listed(ep01) },
          { id: 53, kind: "group" },
        ]);
        await openMember();
        await user.click(await actionsButton());
        expect(await menuTexts()).toEqual(["Turn the group into a tag…", "Dissolve…"]);
      });

      it("「Remove from this group」でその項目を外し、トーストで伝え、group なしで読み直す", async () => {
        const user = userEvent.setup();
        server.groupItems.set(1, [{ id: 52, kind: "video", video: listed(ep02) }]);
        server.grouping.mockImplementation(() => json({ id: 1, name: "series" }));
        renderPage("12?group=1", "/?q=series");
        await ready();
        await screen.findByRole("heading", { level: 2, name: "Up next" });
        await user.click(await actionsButton());
        leaveOnServer();
        await user.click(
          await screen.findByRole("menuitem", { name: "Remove from this group" }),
        );
        expect(await screen.findByText('Removed from "series"')).toBeDefined();
        expect(server.grouping).toHaveBeenCalledWith(
          "POST",
          "/api/groups/1/items/remove",
          { itemIds: [52] },
        );
        await expectPlainVideo();
        expect(videoRequests(12)).toBeGreaterThan(0);
        await waitFor(() =>
          expect(
            fetchMock.mock.calls.some(
              ([input]) => String(input) === "/api/videos/12/related",
            ),
          ).toBe(true),
        );
      });

      it("外す要求の間に別の動画へ移ったら、応答で前の動画へ引き戻さない", async () => {
        const user = userEvent.setup();
        server.groupItems.set(1, [{ id: 52, kind: "video", video: listed(ep02) }]);
        server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
        let answer: ((response: Response) => void) | undefined;
        server.grouping.mockImplementation(
          () =>
            new Promise<Response>((resolve) => {
              answer = resolve;
            }),
        );
        renderPage("12?group=1", "/?q=series");
        await ready();
        await user.click(await actionsButton());
        await user.click(
          await screen.findByRole("menuitem", { name: "Remove from this group" }),
        );
        await waitFor(() => expect(answer).toBeDefined());
        await user.click(screen.getByRole("link", { name: "別の動画" }));
        await waitFor(() =>
          expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
            "後続の動画",
          ),
        );

        await act(async () => {
          answer?.(json({ id: 1, name: "series" }));
          await Promise.resolve();
        });
        expect(await screen.findByText('Removed from "series"')).toBeDefined();
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画");
      });

      it("外す要求の 404 はトーストなしで読み直す", async () => {
        const user = userEvent.setup();
        server.groupItems.set(1, [{ id: 52, kind: "video", video: listed(ep02) }]);
        server.grouping.mockImplementation(() =>
          json({ code: "group_target_not_found", message: "x" }, 404),
        );
        await openMember();
        const before = videoRequests(12);
        await user.click(await actionsButton());
        leaveOnServer();
        await user.click(
          await screen.findByRole("menuitem", { name: "Remove from this group" }),
        );
        await waitFor(() => expect(videoRequests(12)).toBe(before + 1));
        await expectPlainVideo();
        expect(screen.queryByText('Removed from "series"')).toBeNull();
      });

      it("「Turn the group into a tag…」は共有の窓で確かめ、グループの id に POST を送る（受け入れ条件 5）", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ tag: { id: 4, name: "series" }, created: false }),
        );
        await openMember();
        saveListSnapshot(
          { query: "series" },
          { items: [], total: 0, hasMore: false, scrollY: 0 },
        );
        await user.click(await actionsButton());
        await user.click(
          await screen.findByRole("menuitem", { name: "Turn the group into a tag…" }),
        );
        const dialog = await screen.findByRole("alertdialog");
        expect(within(dialog).getByText('Turn "series" into a tag?')).toBeDefined();
        leaveOnServer();
        await user.click(within(dialog).getByRole("button", { name: "Turn into a tag" }));
        expect(
          await screen.findByText('Added the tag "series" and dissolved the group'),
        ).toBeDefined();
        expect(server.grouping).toHaveBeenCalledWith(
          "POST",
          "/api/groups/1/tag",
          undefined,
        );
        await expectPlainVideo();
        expect(screen.queryByRole("alertdialog")).toBeNull();
        expect(takeListSnapshot({ query: "series" })).toBeUndefined();
      });

      it("「Dissolve…」は自動グループのフォルダの文を添えた窓で確かめ、DELETE を送ってページに留まる", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() => new Response(null, { status: 204 }));
        await openMember();
        await user.click(await actionsButton());
        await user.click(await screen.findByRole("menuitem", { name: "Dissolve…" }));
        const dialog = await screen.findByRole("alertdialog");
        expect(within(dialog).getByText('Dissolve "series"?')).toBeDefined();
        expect(
          within(dialog).getByText(/Its folder is no longer grouped automatically/),
        ).toBeDefined();
        leaveOnServer();
        await user.click(within(dialog).getByRole("button", { name: "Dissolve" }));
        expect(await screen.findByText('Dissolved "series"')).toBeDefined();
        expect(server.grouping).toHaveBeenCalledWith(
          "DELETE",
          "/api/groups/1",
          undefined,
        );
        await expectPlainVideo();
        expect(screen.queryByTestId("screen")).toBeNull();
      });

      it("解散の失敗は窓の中に出し、窓を開いたままにする", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ code: "internal", message: "x" }, 500),
        );
        await openMember();
        await user.click(await actionsButton());
        await user.click(await screen.findByRole("menuitem", { name: "Dissolve…" }));
        const dialog = await screen.findByRole("alertdialog");
        await user.click(within(dialog).getByRole("button", { name: "Dissolve" }));
        expect(
          await within(dialog).findByText(/^Couldn't dissolve the group: /),
        ).toBeDefined();
        expect(screen.getByRole("alertdialog")).toBeDefined();
      });

      it("メニューが開いている間の Esc はメニューだけを閉じ、再生画面のままフォーカスをボタンへ戻す", async () => {
        const user = userEvent.setup();
        await openMember();
        const button = await actionsButton();
        button.focus();
        await user.keyboard("{Enter}");
        await screen.findByRole("menu");
        await user.keyboard("{Escape}");
        await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
        expect(screen.queryByTestId("screen")).toBeNull();
        expect(document.activeElement).toBe(button);
        await user.keyboard("{Escape}");
        expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=series");
      });
    });

    describe("開いたグループ（?group）", () => {
      const urls = () => fetchMock.mock.calls.map(([input]) => String(input));

      it("?group=1 なら読み取りが group=1 を運び、予告がそのグループの次の動画を名指し、進んでも ?group=1 が残る", async () => {
        vi.useFakeTimers({ shouldAdvanceTime: true });
        renderPage("12?group=1", "/?q=series");
        await ready();
        await screen.findByRole("heading", { level: 2, name: "Up next" });
        expect(urls()).toContain("/api/videos/12?group=1");
        expect(urls()).toContain("/api/videos/12/related?group=1");
        expect(urls()).not.toContain("/api/videos/12");
        const members = document.getElementById("group-heading")!.closest("section")!;
        expect(
          within(members).getByRole("link", { name: /ep03/ }).getAttribute("href"),
        ).toBe("/videos/13?group=1");
        expect((await groupLine()).getAttribute("href")).toBe("/groups/1");
        await screen.findByTestId("video-player");
        end();
        expect(
          screen
            .getAllByRole("status")
            .filter((node) => node.textContent === announcement),
        ).toHaveLength(1);
        await advance(5000);
        await waitFor(() => expect(player().video.id).toBe(13));
        expect(urls()).toContain("/api/videos/13?group=1");
        expect(urls()).toContain("/api/videos/13/related?group=1");
      });

      it("?group のある矢印は同じ group を保って開く", async () => {
        renderPage("12?group=1", "/?q=series");
        await ready();
        await screen.findByRole("heading", { level: 2, name: "Up next" });
        fireEvent.click(screen.getByRole("button", { name: "Next video: ep03" }));
        await waitFor(() => expect(player().video.id).toBe(13));
        expect(urls()).toContain("/api/videos/13?group=1");
      });

      it("問い合わせが無ければ読み取りは group を運ばない", async () => {
        await openMember();
        await screen.findByRole("link", { name: /ep03/ });
        expect(urls()).toContain("/api/videos/12");
        expect(urls()).toContain("/api/videos/12/related");
        expect(urls().some((url) => url.includes("group="))).toBe(false);
        const members = document.getElementById("group-heading")!.closest("section")!;
        expect(
          within(members).getByRole("link", { name: /ep03/ }).getAttribute("href"),
        ).toBe("/videos/13");
      });
    });

    describe("開いたスマートフォルダ（?smartFolder）", () => {
      const urls = () => fetchMock.mock.calls.map(([input]) => String(input));
      const inFolder = (value: Video, position: number): Video => ({
        ...listed(value),
        smartFolder: { id: 1, name: "タグ: 猫", position, count: 3 },
      });
      const folderOrder = { ...group, name: "タグ: 猫" };

      beforeEach(() => {
        server.videos.set(12, [inFolder(ep02, 2)]);
        server.videos.set(13, [inFolder(ep03, 3)]);
        server.related.set(12, {
          items: [related(8, "後続の動画")],
          nextId: 13,
          prevId: 11,
          group: folderOrder,
        });
        server.related.set(13, {
          items: [related(8, "後続の動画")],
          prevId: 12,
          group: folderOrder,
        });
      });

      const folderLine = () =>
        screen.findByRole("link", {
          name: 'Smart folder "タグ: 猫", video 2 of 3. Open the smart folder',
        });

      it("?smartFolder=1 なら読み取りが smartFolder=1 を運び、予告がフォルダの次の動画を名指し、進んでも ?smartFolder=1 が残る", async () => {
        vi.useFakeTimers({ shouldAdvanceTime: true });
        renderPage("12?smartFolder=1", "/smart-folders/1");
        await ready();
        await screen.findByRole("heading", { level: 2, name: "Up next" });
        expect(urls()).toContain("/api/videos/12?smartFolder=1");
        expect(urls()).toContain("/api/videos/12/related?smartFolder=1");
        expect(urls()).not.toContain("/api/videos/12");
        const members = document.getElementById("group-heading")!.closest("section")!;
        expect(
          within(members).getByRole("link", { name: /ep03/ }).getAttribute("href"),
        ).toBe("/videos/13?smartFolder=1");
        await screen.findByTestId("video-player");
        end();
        expect(
          screen
            .getAllByRole("status")
            .filter((node) => node.textContent === announcement),
        ).toHaveLength(1);
        await advance(5000);
        await waitFor(() => expect(player().video.id).toBe(13));
        expect(urls()).toContain("/api/videos/13?smartFolder=1");
        expect(urls()).toContain("/api/videos/13/related?smartFolder=1");
      });

      it("題名の上のフォルダの行はフォルダのページへのリンクで、後ろに操作を置かない", async () => {
        renderPage("12?smartFolder=1", "/smart-folders/1");
        const link = await folderLine();
        expect(link.textContent).toBe("タグ: 猫·2 / 3");
        expect(link.getAttribute("href")).toBe("/smart-folders/1");
        expect(link.nextElementSibling).toBeNull();
        expect(screen.queryByRole("button", { name: "Group actions" })).toBeNull();
        fireEvent.click(link);
        expect(screen.getByTestId("screen").textContent).toBe(
          "スマートフォルダ /smart-folders/1",
        );
      });

      it("?smartFolder のある矢印は同じ smartFolder を保って開く", async () => {
        renderPage("12?smartFolder=1", "/smart-folders/1");
        await ready();
        await screen.findByRole("heading", { level: 2, name: "Up next" });
        fireEvent.click(screen.getByRole("button", { name: "Next video: ep03" }));
        await waitFor(() => expect(player().video.id).toBe(13));
        expect(urls()).toContain("/api/videos/13?smartFolder=1");
      });

      it("?group と ?smartFolder の両方があれば group だけを運ぶ", async () => {
        renderPage("12?group=1&smartFolder=1", "/?q=series");
        await ready();
        await screen.findByRole("heading", { level: 2, name: "Up next" });
        expect(urls()).toContain("/api/videos/12?group=1");
        expect(urls().some((url) => url.includes("smartFolder="))).toBe(false);
      });

      it("ゲストにも同じフォルダの行を出す", async () => {
        server.session = "guest";
        renderPage("12?smartFolder=1", "/smart-folders/1", "guest");
        expect((await folderLine()).getAttribute("href")).toBe("/smart-folders/1");
      });
    });

    it("グループに属さない動画は題名の上に何も出さない", async () => {
      renderPage();
      const title = await ready();
      expect(title.previousElementSibling).toBeNull();
    });

    it("関連動画の列の上位にメンバーを順に並べ、今のメンバーを示し、境目の下にメンバーを出さない（受け入れ条件 15）", async () => {
      await openMember();
      const heading = screen.getByRole("heading", { level: 2, name: "Up next" });
      const section = heading.closest("section");
      if (section === null) throw new Error("列がありません");
      const [members, others] = within(section).getAllByRole("list") as [
        HTMLElement,
        HTMLElement,
      ];
      expect(
        within(members)
          .getAllByRole("listitem")
          .map((row) => row.getAttribute("aria-current") === "true"),
      ).toEqual([false, true, false]);
      expect(within(members).getByRole("link", { name: /ep01/ })).toBeDefined();
      expect(within(members).getByRole("link", { name: /ep03/ })).toBeDefined();
      expect(
        within(others)
          .getAllByRole("link")
          .map((link) => link.textContent),
      ).toEqual(["Preparing4:02後続の動画"]);
      expect(within(others).queryByRole("link", { name: /ep0/ })).toBeNull();
    });

    it("前後のつまみはグループの中の前後で、題名をメンバーの並びから引く", async () => {
      await openMember();
      expect(screen.getByRole("button", { name: "Previous video: ep01" })).toBeDefined();
      expect(screen.getByRole("button", { name: "Next video: ep03" })).toBeDefined();
    });

    it("再生が終わると予告を一度だけ読み上げ、5 秒後に確かめてから次のメンバーを再生する（受け入れ条件 16・19）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      (await screen.findByRole("button", { name: /^Next video/ })).focus();
      end();
      const statuses = screen
        .getAllByRole("status")
        .filter((node) => node.textContent === announcement);
      expect(statuses).toHaveLength(1);
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" }));
      const cancel = screen.getByRole("button", { name: "Cancel" });
      const now = screen.getByRole("button", { name: "Play now" });
      expect(
        cancel.compareDocumentPosition(now) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      const seconds = screen.getByText("in 5 seconds");
      expect(seconds.getAttribute("aria-hidden")).toBe("true");
      await advance(2000);
      expect(screen.getByText("in 3 seconds")).toBeDefined();
      expect(
        screen.getAllByRole("status").filter((node) => node.textContent === announcement),
      ).toHaveLength(1);
      const before = videoRequests(13);
      await advance(3000);
      await waitFor(() => expect(player().video.id).toBe(13));
      expect(videoRequests(13)).toBeGreaterThan(before);
      expect(player().autoplay).toBe(true);
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep03");
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=series");
    });

    it("隠れたタブでタイマーが間引かれても、期限を過ぎていれば見えたときに次のメンバーを再生する", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      await screen.findByTestId("video-player");
      end();
      expect(screen.getByText("in 5 seconds")).toBeDefined();
      // Simulate 30 seconds passing without any timer callbacks.
      const now = performance.now.bind(performance);
      const skipped = now() + 30_000;
      const clock = vi.spyOn(performance, "now").mockImplementation(() => skipped);
      try {
        act(() => {
          document.dispatchEvent(new Event("visibilitychange"));
        });
        await waitFor(() => expect(player().video.id).toBe(13));
        expect(player().autoplay).toBe(true);
      } finally {
        clock.mockRestore();
      }
    });

    it("「取り消す」で今の再生終了の層に戻り、フォーカスを「次を再生」へ移す", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      (await screen.findByRole("button", { name: /^Next video/ })).focus();
      end();
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(screen.queryByText(announcement)).toBeNull();
      expect(screen.getByRole("status").textContent).toBe("Playback finished");
      const playNext = screen.getByRole("button", { name: "Play next" });
      expect(document.activeElement).toBe(playNext);
      expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
      await advance(6000);
      expect(player().video.id).toBe(12);
    });

    it("予告中の Esc は取り消しで画面を閉じず、取り消した後の Esc は閉じる", async () => {
      await openMember();
      await screen.findByTestId("video-player");
      end();
      fireEvent.keyDown(screen.getByRole("button", { name: "Cancel" }), {
        key: "Escape",
      });
      expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
      expect(screen.getByRole("button", { name: "Play next" })).toBeDefined();
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep02");
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=series");
    });

    it("「今すぐ再生」で戻り先付きで次のメンバーへ移る", async () => {
      await openMember();
      await screen.findByTestId("video-player");
      end();
      fireEvent.click(screen.getByRole("button", { name: "Play now" }));
      await waitFor(() => expect(player().video.id).toBe(13));
      expect(player().autoplay).toBe(true);
    });

    it("最後のメンバーでは予告を出さず、「もう一度見る」だけの層を出す（受け入れ条件 16）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember("13");
      await screen.findByTestId("video-player");
      end();
      expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
      expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "Play next" })).toBeNull();
      await advance(6000);
      expect(player().video.id).toBe(13);
    });

    it("グループに属さない動画は、終わっても予告を出さず自動で進まない（受け入れ条件 17）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      end();
      expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
      expect(screen.getByRole("button", { name: "Play next" })).toBeDefined();
      await advance(6000);
      expect(player().video.id).toBe(7);
    });

    it("予告の終わりに次のメンバーが無ければ、先へ進まず「もう一度見る」だけの層にする", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      await screen.findByTestId("video-player");
      end();
      server.videos.delete(13);
      await advance(5000);
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull(),
      );
      expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "Play next" })).toBeNull();
      expect(player().video.id).toBe(12);
    });

    it("予告中に次のメンバーが消えた知らせが届いたら、確かめて予告をやめる", async () => {
      await openMember();
      await screen.findByTestId("video-player");
      end();
      server.videos.delete(13);
      const before = videoRequests(13);
      await emitServerEvent("video", { id: 8 });
      expect(videoRequests(13)).toBe(before);
      await emitServerEvent("video", { id: 13 });
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull(),
      );
      expect(screen.getByRole("button", { name: "Watch again" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "Play next" })).toBeNull();
    });

    it("自動で開いたメンバーが再生に失敗したら、再生失敗の層で止まり先へ進まない", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      await screen.findByTestId("video-player");
      end();
      await advance(5000);
      await waitFor(() => expect(player().video.id).toBe(13));
      act(() => player().onError(12_000, "source"));
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("Couldn't play this video")).toBeDefined();
      await advance(6000);
      expect(player().video.id).toBe(13);
      expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    });

    it("メンバーの並びから別のメンバーへ移っても、Esc で開く前の一覧へ戻る（受け入れ条件 19）", async () => {
      await openMember();
      fireEvent.click(screen.getByRole("link", { name: /ep01/ }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep01"),
      );
      await screen.findByRole("heading", { level: 2, name: "Up next" });
      fireEvent.click(screen.getByRole("link", { name: /ep03/ }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep03"),
      );
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=series");
    });

    it("ゲストにも公開のメンバーの並びと予告を出す", async () => {
      renderPage("12", "/", "guest");
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Up next" });
      await screen.findByTestId("video-player");
      end();
      expect(screen.getByRole("button", { name: "Cancel" })).toBeDefined();
    });
  });

  describe("途切れの警告", () => {
    function report(overrides: Partial<PlayerStatus> = {}) {
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: true,
          positioned: true,
          ...overrides,
        }),
      );
    }

    function warning() {
      return screen.queryByText("Slow connection is interrupting playback");
    }

    it("stalled で左上に知らせるだけの警告を出し、画質を変える操作を置かない", async () => {
      renderPage();
      await ready();
      await screen.findByTestId("video-player");
      expect(warning()).toBeNull();
      report();
      const status = warning()?.closest("[role=status]");
      if (!(status instanceof HTMLElement)) throw new Error("警告がありません");
      expect(
        within(status)
          .getAllByRole("button")
          .map((b) => b.ariaLabel),
      ).toEqual(["Dismiss"]);
      expect(within(status).queryByText(/Quality|480p|720p|360p/)).toBeNull();
      const layer = status.closest("[data-stall-warning]");
      expect(layer?.className).toContain("pointer-events-none");
      expect(layer?.className).toContain("inset-x-2");
      expect(layer?.closest("[data-overlay-layer]")).toBeNull();
      expect(within(status).getByRole("button", { name: "Dismiss" }).className).toContain(
        "pointer-events-auto",
      );
    });

    it("閉じると消え、同じ動画では再び出さず、別の動画へ移ると閉じた記録を忘れる", async () => {
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      server.related.set(8, { items: [related(7, "テスト動画")], prevId: 7 });
      renderPage();
      await ready();
      report();
      fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
      expect(warning()).toBeNull();
      report({ stalled: false });
      report();
      expect(warning()).toBeNull();

      fireEvent.click(
        await screen.findByRole("button", { name: "Next video: 後続の動画" }),
      );
      await waitFor(() => expect(player().video.id).toBe(8));
      expect(warning()).toBeNull();
      report();
      expect(warning()).not.toBeNull();
    });

    it("失敗から再試行しても、閉じた警告を出し直さない", async () => {
      renderPage();
      await ready();
      report();
      fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
      act(() => player().onError(1000, "network"));
      fireEvent.click(await screen.findByRole("button", { name: /Try again/ }));
      report();
      expect(warning()).toBeNull();
    });

    it("再生終了・再接続中・失敗の層が出ている間は警告を出さない", async () => {
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      report({ ended: true, playing: false });
      expect(
        screen.getByRole("heading", { level: 2, name: "Playback finished" }),
      ).toBeDefined();
      expect(warning()).toBeNull();
      report({ reconnecting: true, playing: false });
      expect(screen.getByText("Connection lost · Reconnecting")).toBeDefined();
      expect(warning()).toBeNull();
      report();
      expect(warning()).not.toBeNull();
      act(() => player().onError(1000, "decode"));
      await screen.findByRole("alert");
      expect(warning()).toBeNull();
    });

    it("データ待ちの読み込み中と同時なら、読み込み中の表示と警告を並べて出す", async () => {
      renderPage();
      await ready();
      report({ loading: true });
      const texts = screen.getAllByRole("status").map((status) => status.textContent);
      expect(texts).toContain("Loading");
      expect(texts).toContain("Slow connection is interrupting playback");
    });
  });

  describe("再生位置の保存", () => {
    it("playerの即時保存通知をprogress APIへ送る", async () => {
      renderPage();
      await ready();
      player().onProgress(12_345, true);
      await waitFor(() => {
        expect(
          fetchMock.mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/videos/7/progress" &&
              init?.method === "PUT" &&
              init.body === JSON.stringify({ positionMs: 12_345 }),
          ),
        ).toBe(true);
      });
    });

    it("最初の再生で、positioned のあとに視聴の識別子を progress API の body へ送る", async () => {
      renderPage();
      await ready();
      // Autoplay may emit play before the resume seek completes.
      act(() => player().onPlay?.());
      act(() => player().onPosition(42_000));
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      await waitFor(() => {
        const bodies = fetchMock.mock.calls
          .filter(
            ([input, init]) =>
              String(input) === "/api/videos/7/progress" && init?.method === "PUT",
          )
          .map(([, init]) => JSON.parse(String(init?.body)) as Record<string, unknown>);
        expect(bodies).toEqual([
          { positionMs: 42_000, playbackId: expect.stringMatching(/^[0-9a-f-]{36}$/) },
        ]);
      });
    });

    it("位置を知らせる前に positioned で再生しても、最初の位置で視聴の識別子を送る", async () => {
      renderPage();
      await ready();
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: false,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
        }),
      );
      act(() => player().onPlay?.());
      act(() => player().onPosition(0));
      await waitFor(() => {
        const bodies = fetchMock.mock.calls
          .filter(
            ([input, init]) =>
              String(input) === "/api/videos/7/progress" && init?.method === "PUT",
          )
          .map(([, init]) => JSON.parse(String(init?.body)) as Record<string, unknown>);
        expect(bodies).toEqual([
          { positionMs: 0, playbackId: expect.stringMatching(/^[0-9a-f-]{36}$/) },
        ]);
      });
    });

    it("アンマウントでも最後の再生位置を送る", async () => {
      const page = renderPage();
      await ready();
      player().onPosition(12_345);
      page.unmount();
      const finalCall = fetchMock.mock.calls
        .filter(
          ([input, init]) =>
            String(input) === "/api/videos/7/progress" && init?.keepalive === true,
        )
        .at(-1);
      expect(finalCall?.[1]).toEqual(
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ positionMs: 12_345 }),
          keepalive: true,
        }),
      );
    });

    it("別動画へのroute変更で前の動画の進捗を新しいIDへ送らない", async () => {
      server.videos.set(8, () => new Promise(() => undefined) as never);
      const page = renderPage();
      await ready();
      player().onPosition(12_345);
      fireEvent.click(screen.getByRole("link", { name: "別の動画" }));
      await waitFor(() => expect(screen.queryByTestId("video-player")).toBeNull());
      page.unmount();
      const finalProgressURLs = fetchMock.mock.calls
        .filter(([, init]) => init?.keepalive === true)
        .map(([input]) => String(input));
      expect(finalProgressURLs).toEqual(["/api/videos/7/progress"]);
    });

    it("無効なrouteへ変更したら前の動画の再生エラーを消す", async () => {
      renderPage();
      await ready();
      act(() => player().onError(1000, "source"));
      expect(await screen.findByText("Couldn't play this video")).toBeDefined();
      fireEvent.click(screen.getByRole("link", { name: "無効な動画" }));
      expect(await screen.findByText("This video can't be opened")).toBeDefined();
      expect(screen.queryByText("Couldn't play this video")).toBeNull();
    });
  });
  describe("公開の切り替え", () => {
    function holdVisibility() {
      const answers: ((response: Response) => void)[] = [];
      const bodies: unknown[] = [];
      const answered = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        if (String(input) !== "/api/video-visibility") return answered!(input, init);
        const body = JSON.parse(String(init?.body)) as {
          videoIds: number[];
          public: boolean;
        };
        bodies.push(body);
        return new Promise<Response>((resolve) =>
          answers.push((response) => {
            if (response.ok) {
              for (const id of body.videoIds) {
                const entry = server.videos.get(id);
                if (!Array.isArray(entry)) continue;
                const last = entry[entry.length - 1]!;
                server.videos.set(id, [{ ...last, public: body.public }]);
              }
            }
            resolve(response);
          }),
        );
      });
      return { answers, bodies };
    }

    function toggle() {
      return screen.getByRole("switch", { name: "Show to people who aren't signed in" });
    }

    /** The state's word before the track, in the same label. */
    function word() {
      return toggle().closest("label")?.textContent;
    }

    it("所有者には非公開の状態で出し、押すと1回だけ送って応答の後に公開中へ変わる", async () => {
      const { answers, bodies } = holdVisibility();
      renderPage();
      await ready();
      expect(toggle().getAttribute("aria-checked")).toBe("false");
      expect(word()).toBe("Private");

      fireEvent.click(toggle());
      expect(toggle().getAttribute("aria-disabled")).toBe("true");
      expect(toggle().getAttribute("aria-checked")).toBe("false");
      fireEvent.click(toggle());
      expect(bodies).toEqual([{ videoIds: [7], public: true }]);

      await act(async () => answers[0]!(json({ applied: 1 })));
      await waitFor(() => expect(toggle().getAttribute("aria-checked")).toBe("true"));
      expect(word()).toBe("Public");
      expect(toggle().getAttribute("aria-disabled")).toBeNull();
      expect(screen.queryByRole("status")).toBeNull();
    });

    it("公開中を押すと public: false を送り、非公開に戻る", async () => {
      server.videos.set(7, [{ ...video, public: true }]);
      const { answers, bodies } = holdVisibility();
      renderPage();
      await ready();
      expect(word()).toBe("Public");
      // Pressing the word toggles as the track does.
      fireEvent.click(screen.getByText("Public"));
      expect(bodies).toEqual([{ videoIds: [7], public: false }]);
      await act(async () => answers[0]!(json({ applied: 1 })));
      await waitFor(() => expect(toggle().getAttribute("aria-checked")).toBe("false"));
    });

    it("失敗したら状態を変えずに理由を出し、次に押すと消える", async () => {
      const { answers } = holdVisibility();
      renderPage();
      await ready();

      fireEvent.click(toggle());
      await act(async () =>
        answers[0]!(json({ code: "internal", message: "失敗" }, 500)),
      );
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe(
        "Couldn't change the visibility: Something went wrong on the server.",
      );
      expect(toggle().getAttribute("aria-checked")).toBe("false");

      fireEvent.click(toggle());
      expect(screen.queryByRole("alert")).toBeNull();
      // Resolve serialized toggles so later tests do not wait behind pending requests.
      await act(async () => answers[1]!(json({ applied: 1 })));
      await waitFor(() => expect(toggle().getAttribute("aria-checked")).toBe("true"));
    });

    it("別の動画へ移ると失敗の行を持ち越さない", async () => {
      server.videos.set(8, [{ ...video, id: 8, title: "後続の動画" }]);
      const { answers } = holdVisibility();
      renderPage();
      await ready();
      fireEvent.click(toggle());
      await act(async () =>
        answers[0]!(json({ code: "internal", message: "失敗" }, 500)),
      );
      await screen.findByRole("alert");

      fireEvent.click(screen.getByRole("link", { name: "別の動画" }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画"),
      );
      expect(screen.queryByRole("alert")).toBeNull();
    });

    it("ゲストには切り替えを出さない", async () => {
      server.videos.set(7, [{ ...video, location: undefined, public: true }]);
      renderPage("7", undefined, "guest");
      await ready();
      expect(screen.queryByRole("switch")).toBeNull();
    });
  });

  describe("お気に入り", () => {
    const owned: Video = { ...video, favorite: false };

    function holdFavorites() {
      const answers: ((response: Response) => void)[] = [];
      const bodies: unknown[] = [];
      const answered = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        if (String(input) !== "/api/favorites") return answered!(input, init);
        const body = JSON.parse(String(init?.body)) as {
          videoIds: number[];
          groupIds: unknown[];
          favorite: boolean;
        };
        bodies.push(body);
        return new Promise<Response>((resolve) =>
          answers.push((response) => {
            if (response.ok) {
              for (const id of body.videoIds) {
                const entry = server.videos.get(id);
                if (!Array.isArray(entry)) continue;
                const last = entry[entry.length - 1]!;
                server.videos.set(id, [{ ...last, favorite: body.favorite }]);
              }
            }
            resolve(response);
          }),
        );
      });
      return { answers, bodies };
    }

    function favoriteButton() {
      return screen.getByRole("button", { name: "Favorite" });
    }

    function videoRequests(id: number) {
      return fetchMock.mock.calls.filter(
        ([input, init]) =>
          String(input) === `/api/videos/${String(id)}` &&
          (init?.method ?? "GET") === "GET",
      ).length;
    }

    const applied = () => json({ appliedVideos: 1, appliedGroups: 0 });

    it("題名の後の最初の操作として文字付きで置く", async () => {
      server.videos.set(7, [owned]);
      renderPage();
      await ready();
      const button = favoriteButton();
      const title = screen.getByRole("heading", { level: 1 });
      expect(button.parentElement?.firstElementChild).toBe(button);
      expect(
        title.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      expect(button.textContent).toBe("Favorite");
      expect(button.getAttribute("aria-pressed")).toBe("false");
      expect(button.getAttribute("data-state")).toBe("off");
      // The three title actions are labelled pills, never ghost words.
      for (const name of ["Favorite", "Playlist", "Group"]) {
        expect(screen.getByRole("button", { name }).getAttribute("data-shape")).toBe(
          "pill",
        );
      }
    });

    it("所在が無くプレイヤーの出ていない動画でも、付け外しを出す", async () => {
      server.videos.set(7, [
        { ...owned, location: undefined, probeState: "failed", playable: false },
      ]);
      renderPage();
      await ready();
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("false");
    });

    it("押すと videoIds: [id] を 1 回だけ送り、応答の後に取り直して塗りに変わる（受け入れ条件 1）", async () => {
      server.videos.set(7, [owned]);
      const { answers, bodies } = holdFavorites();
      renderPage();
      await ready();
      const before = videoRequests(7);

      fireEvent.click(favoriteButton());
      expect(favoriteButton().getAttribute("aria-disabled")).toBe("true");
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("false");
      fireEvent.click(favoriteButton());
      expect(bodies).toEqual([{ videoIds: [7], groupIds: [], favorite: true }]);

      await act(async () => answers[0]!(applied()));
      await waitFor(() => expect(videoRequests(7)).toBe(before + 1));
      await waitFor(() =>
        expect(favoriteButton().getAttribute("aria-pressed")).toBe("true"),
      );
      expect(favoriteButton().getAttribute("aria-disabled")).toBeNull();
      expect(favoriteButton().getAttribute("data-state")).toBe("on");
      expect(favoriteButton().className).toContain("data-[state=on]:bg-favorite-soft");
      expect(favoriteButton().className).toContain("data-[state=on]:text-favorite");
      expect(favoriteButton().querySelector("svg")?.getAttribute("class")).toContain(
        "fill-current",
      );
      expect(screen.queryByRole("status")).toBeNull();
      expect(screen.queryByRole("alert")).toBeNull();

      fireEvent.click(favoriteButton());
      expect(bodies[1]).toEqual({ videoIds: [7], groupIds: [], favorite: false });
      await act(async () => answers[1]!(applied()));
      await waitFor(() =>
        expect(favoriteButton().getAttribute("aria-pressed")).toBe("false"),
      );
    });

    it("失敗したら状態を変えずに題名の行の直下に理由を出し、次に押すと消える", async () => {
      server.videos.set(7, [owned]);
      const { answers } = holdFavorites();
      renderPage();
      await ready();

      fireEvent.click(favoriteButton());
      await act(async () =>
        answers[0]!(json({ code: "internal", message: "失敗" }, 500)),
      );
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe(
        "Couldn't change the favorite: Something went wrong on the server.",
      );
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("false");
      expect(favoriteButton().getAttribute("aria-disabled")).toBeNull();
      expect(screen.queryByRole("status")).toBeNull();

      fireEvent.click(favoriteButton());
      expect(screen.queryByRole("alert")).toBeNull();
      await act(async () => answers[1]!(applied()));
      await waitFor(() =>
        expect(favoriteButton().getAttribute("aria-pressed")).toBe("true"),
      );
    });

    it("付け外しの成功後も、取り直しが終わるまでと取り直しに失敗したときは前の状態のまま", async () => {
      server.videos.set(7, [owned]);
      const { answers } = holdFavorites();
      renderPage();
      await ready();
      const before = videoRequests(7);
      server.videos.set(7, () => json({ code: "internal", message: "失敗" }, 500));

      fireEvent.click(favoriteButton());
      await act(async () => answers[0]!(applied()));
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("false");
      await waitFor(() => expect(videoRequests(7)).toBe(before + 1));
      await waitFor(() =>
        expect(favoriteButton().getAttribute("aria-disabled")).toBeNull(),
      );
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("false");
      expect(favoriteButton().getAttribute("data-active")).toBeNull();
    });

    it("別の動画へ移ると失敗の行を持ち越さない", async () => {
      server.videos.set(7, [owned]);
      server.videos.set(8, [{ ...owned, id: 8, title: "後続の動画" }]);
      const { answers } = holdFavorites();
      renderPage();
      await ready();
      fireEvent.click(favoriteButton());
      await act(async () =>
        answers[0]!(json({ code: "internal", message: "失敗" }, 500)),
      );
      await screen.findByRole("alert");

      fireEvent.click(screen.getByRole("link", { name: "別の動画" }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画"),
      );
      expect(screen.queryByRole("alert")).toBeNull();
    });

    it("付けた結果は一覧の控えにも反映され、戻ったカードの印が変わる（受け入れ条件 1）", async () => {
      server.videos.set(7, [owned]);
      const { answers } = holdFavorites();
      saveListSnapshot(
        { query: "" },
        {
          items: [
            { kind: "video", video: { ...owned, favorite: true } },
            { kind: "video", video: { ...owned, id: 8, favorite: true } },
          ],
          total: 2,
          hasMore: false,
          scrollY: 0,
        },
      );
      server.videos.set(7, [{ ...owned, favorite: true }]);
      renderPage();
      await ready();
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("true");

      fireEvent.click(favoriteButton());
      await act(async () => answers[0]!(applied()));
      await waitFor(() =>
        expect(favoriteButton().getAttribute("aria-pressed")).toBe("false"),
      );
      const snapshot = takeListSnapshot({ query: "" });
      expect(
        snapshot?.items.map((item) =>
          item.kind === "video" ? [item.video.id, item.video.favorite] : undefined,
        ),
      ).toEqual([
        [7, false],
        [8, true],
      ]);
    });

    it("一覧側で変えた結果を、取り直さずに再生画面の 1 件へ重ねる", async () => {
      server.videos.set(7, [owned]);
      const { answers } = holdFavorites();
      renderPage();
      await ready();
      const before = videoRequests(7);

      let done: Promise<unknown> | undefined;
      act(() => {
        done = updateFavorites([7], [], true);
      });
      await act(async () => {
        answers[0]!(applied());
        await done;
      });
      await waitFor(() =>
        expect(favoriteButton().getAttribute("aria-pressed")).toBe("true"),
      );
      expect(videoRequests(7)).toBe(before);
    });

    it("見終わった動画でも印が残り、先頭から再生する（受け入れ条件 6）", async () => {
      server.videos.set(7, [
        {
          ...owned,
          favorite: true,
          progress: {
            positionMs: 242_000,
            completed: true,
            updatedAt: "2026-09-02T00:00:00Z",
          },
        },
      ]);
      renderPage();
      await ready();
      expect(favoriteButton().getAttribute("aria-pressed")).toBe("true");
      expect(player().initialPositionMs).toBe(0);
    });

    it("グループのメンバーでも、グループの行にお気に入りを置かない", async () => {
      server.videos.set(7, [
        {
          ...owned,
          group: {
            id: 1,
            folder: { rootId: 1, path: "series" },
            name: "series",
            position: 1,
            count: 3,
          },
        },
      ]);
      renderPage();
      await ready();
      expect(screen.getAllByRole("button", { name: /Favorite/ })).toHaveLength(1);
    });

    it("ゲストには印も付け外しも出さない（受け入れ条件 10）", async () => {
      server.videos.set(7, [{ ...video, location: undefined, public: true }]);
      renderPage("7", undefined, "guest");
      await ready();
      expect(screen.queryByRole("button", { name: "Favorite" })).toBeNull();
      expect(screen.queryByRole("button", { name: /Favorite/ })).toBeNull();
    });
  });

  describe("プレイリストに足す", () => {
    const owned: Video = { ...video, favorite: false };

    it("お気に入りの直後、グループに足すの前に置き、選ぶとこの動画の id を送る", async () => {
      server.videos.set(7, [
        {
          ...owned,
          group: {
            id: 1,
            folder: { rootId: 1, path: "series" },
            name: "series",
            position: 1,
            count: 3,
          },
        },
      ]);
      const bodies: unknown[] = [];
      const answered = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        const url = String(input);
        if (url.startsWith("/api/playlists?")) {
          return Promise.resolve(
            json({
              items: [
                {
                  id: 5,
                  name: "Tonight",
                  itemCount: 1,
                  updatedAt: "2026-10-01T00:00:00Z",
                },
              ],
              total: 1,
              version: "1",
            }),
          );
        }
        if (url === "/api/playlists/5/items") {
          bodies.push(JSON.parse(String(init?.body)));
          return Promise.resolve(json({ added: 1, existing: 0 }));
        }
        return answered!(input, init);
      });
      renderPage();
      await ready();
      const buttons = screen.getAllByRole("button", { name: "Playlist" });
      expect(buttons).toHaveLength(1);
      const add = buttons[0]!;
      const favorite = screen.getByRole("button", { name: "Favorite" });
      expect(favorite.nextElementSibling).toBe(add);
      const addToGroup = screen.getByRole("button", { name: "Group" });
      expect(add.nextElementSibling).toBe(addToGroup);

      fireEvent.click(add);
      fireEvent.click(await screen.findByRole("option", { name: "Tonight" }));
      expect(await screen.findByText('Added 1 item to "Tonight"')).toBeDefined();
      expect(bodies).toEqual([{ videoIds: [7], groupIds: [] }]);
    });

    it("ゲストには出さず、プレイリストを読まない（受け入れ条件 9）", async () => {
      server.videos.set(7, [{ ...video, location: undefined, public: true }]);
      renderPage("7", undefined, "guest");
      await ready();
      expect(screen.queryByRole("button", { name: "Playlist" })).toBeNull();
      expect(
        fetchMock.mock.calls.some(([input]) =>
          String(input).startsWith("/api/playlists"),
        ),
      ).toBe(false);
    });
  });

  describe("表示名", () => {
    const fileTitle = "テスト動画";
    const named: Video = {
      ...video,
      title: "夏の旅行",
      fileTitle,
      displayName: "夏の旅行",
    };

    it("所有者の題名にも編集のボタンは無く、表示名のある動画は元のファイル名を従の行で見せる（受け入れ条件 7）", async () => {
      server.videos.set(7, [named]);
      renderPage();
      await ready();
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("夏の旅行");
      expect(screen.getByTitle(`File name: ${fileTitle}`).textContent).toBe(
        `File name ${fileTitle}`,
      );
      expect(screen.queryByRole("button", { name: "Edit name" })).toBeNull();
      expect(screen.getByRole("link", { name: "Edit" })).toBeDefined();
    });

    it("ゲストの動画ページには編集の入口もファイル名の行も無い（受け入れ条件 9）", async () => {
      server.videos.set(7, [
        { ...video, title: "夏の旅行", location: undefined, public: true },
      ]);
      renderPage("7", undefined, "guest");
      await ready();
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("夏の旅行");
      expect(screen.queryByRole("button", { name: "Edit name" })).toBeNull();
      expect(screen.queryByTitle(/^File name/)).toBeNull();
    });
  });

  describe("更新日時", () => {
    // Use noon to avoid a date change in the browser's timezone.
    const before = new Date(2026, 8, 1, 12, 0).toISOString();
    const after = new Date(2026, 8, 28, 12, 0).toISOString();
    const tag = {
      id: 5,
      name: "旅行",
      manual: true,
      fromFolder: false,
      tentative: false,
      kind: null,
    };

    let current: Video;
    let failRefetch = false;

    beforeEach(() => {
      current = { ...video, addedAt: before, updatedAt: before };
      failRefetch = false;
      server.videos.set(7, () =>
        failRefetch ? json({ code: "internal", message: "失敗" }, 500) : json(current),
      );
      const answered = fetchMock.getMockImplementation()!;
      fetchMock.mockImplementation((input, init) => {
        const url = String(input);
        if (url === "/api/video-tags" && init?.method === "POST") {
          const body = JSON.parse(String(init.body)) as {
            action: "add" | "remove";
            tag: { id: number } | { name: string };
          };
          const tags = body.action === "add" ? [tag] : [];
          current = { ...current, tags, updatedAt: after };
          return Promise.resolve(
            json({ tag: { id: tag.id, name: tag.name }, applied: 1 }),
          );
        }
        if (url === "/api/video-visibility") {
          current = { ...current, public: !current.public, updatedAt: after };
          return Promise.resolve(json({ applied: 1 }));
        }
        return answered(input, init);
      });
    });

    /** The panel stays open across the refetches, as the selected tab is kept. */
    async function showFile() {
      await ready();
      openTab("File & media");
    }

    /** The value of a File fact, by its term. */
    function fileFact(term: string): string {
      const label = within(screen.getByLabelText("File")).getByText(term, {
        selector: "dt",
      });
      return label.nextElementSibling?.textContent ?? "";
    }

    function edited(): string {
      return fileFact("Edited in vv");
    }

    async function expectEditedAfter() {
      await waitFor(() => expect(edited()).toBe("Sep 28, 2026"));
    }

    it("一度も編集していない動画は追加日と同じ日付で、所有者にもゲストにも更新日時と作成日時を出す（受け入れ条件 3）", async () => {
      renderPage();
      await showFile();
      expect(edited()).toBe("Sep 1, 2026");
      expect(edited()).toBe(fileFact("Added"));
      expect(fileFact("File created")).not.toBe("");
      expect(fileFact("File modified")).not.toBe("");
    });

    it("ゲストにも更新日時と作成日時を出す", async () => {
      server.session = "guest";
      current = { ...current, location: undefined, public: true };
      renderPage("7", undefined, "guest");
      await showFile();
      expect(edited()).toBe("Sep 1, 2026");
      expect(fileFact("File created")).not.toBe("");
      expect(fileFact("File modified")).not.toBe("");
    });

    it("タグを付けると動画を取り直して更新日時が進む（受け入れ条件 1）", async () => {
      const user = userEvent.setup();
      renderPage();
      await showFile();
      const input = screen.getByRole("combobox", { name: "Add tag" });
      await user.click(input);
      await user.type(input, "旅行{Enter}");
      await expectEditedAfter();
    });

    it("タグを外すと動画を取り直して更新日時が進む（受け入れ条件 1）", async () => {
      current = { ...current, tags: [tag] };
      renderPage();
      await showFile();
      fireEvent.click(
        await screen.findByRole("button", { name: "Remove 旅行 from this video" }),
      );
      await expectEditedAfter();
    });

    it("公開を切り替えると動画を取り直して更新日時が進む（受け入れ条件 1）", async () => {
      renderPage();
      await showFile();
      fireEvent.click(
        screen.getByRole("switch", { name: "Show to people who aren't signed in" }),
      );
      await expectEditedAfter();
      expect(
        screen
          .getByRole("switch", { name: "Show to people who aren't signed in" })
          .getAttribute("aria-checked"),
      ).toBe("true");
    });

    it("取り直しが失敗しても失敗の行を出さず、前の値のまま置く", async () => {
      renderPage();
      await showFile();
      failRefetch = true;
      fireEvent.click(
        screen.getByRole("switch", { name: "Show to people who aren't signed in" }),
      );
      await waitFor(() =>
        expect(
          screen
            .getByRole("switch", { name: "Show to people who aren't signed in" })
            .getAttribute("aria-checked"),
        ).toBe("true"),
      );
      await waitFor(() =>
        expect(
          fetchMock.mock.calls.filter(([input]) => String(input) === "/api/videos/7"),
        ).toHaveLength(2),
      );
      await act(async () => {
        await Promise.resolve();
      });
      expect(edited()).toBe("Sep 1, 2026");
      expect(screen.queryByRole("alert")).toBeNull();
    });
  });

  describe("見る人", () => {
    function guestVideo(overrides: Partial<Video> = {}): Video {
      // Guest responses omit location, progress, and probeError and return no tags.
      return { ...video, location: undefined, tags: [], public: true, ...overrides };
    }

    it("ゲストにはタグ・ファイルの操作を出さず、再生位置を送らない", async () => {
      server.videos.set(7, [guestVideo()]);
      const { unmount } = renderPage("7", undefined, "guest");
      await ready();
      openTab("File & media");
      expect(screen.getByRole("region", { name: "Media" })).toBeDefined();
      expect(screen.queryByRole("region", { name: "Location" })).toBeNull();
      expect(screen.queryByRole("heading", { name: "Tags" })).toBeNull();
      expect(screen.queryByPlaceholderText("Add tag")).toBeNull();
      expect(screen.queryByRole("button", { name: /Open file/ })).toBeNull();
      expect(screen.queryByRole("button", { name: "Copy path" })).toBeNull();

      act(() => player().onProgress(30_000, true));
      act(() => player().onPosition(31_000));
      unmount();
      const progressCalls = fetchMock.mock.calls.filter(([input]) =>
        String(input).endsWith("/progress"),
      );
      expect(progressCalls).toHaveLength(0);
    });

    it("ゲストには仮のタグも確定したタグも出さない（031 受け入れ条件4）", async () => {
      server.videos.set(7, [
        guestVideo({
          tags: [
            {
              kind: null,
              id: 1,
              name: "高画質",
              manual: true,
              fromFolder: false,
              tentative: true,
            },
            {
              kind: null,
              id: 2,
              name: "旅行",
              manual: true,
              fromFolder: false,
              tentative: false,
            },
          ],
        }),
      ]);
      renderPage("7", undefined, "guest");
      await ready();
      expect(screen.queryByRole("heading", { name: "Tags" })).toBeNull();
      expect(screen.queryByRole("link", { name: /^Filter by/ })).toBeNull();
      expect(document.querySelector("svg.lucide-circle-dashed")).toBeNull();
    });

    it("所有者にはタグの並びとファイルの操作を出す", async () => {
      renderPage();
      await ready();
      expect(screen.getByRole("heading", { name: "Tags" })).toBeDefined();
      openTab("File & media");
      expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
    });

    it("ゲストの読み取り失敗には、やり直し・ファイルを開く・誤りの文を出さない", async () => {
      server.videos.set(7, [guestVideo({ probeState: "failed" })]);
      renderPage("7", undefined, "guest");
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("Couldn't read this video")).toBeDefined();
      expect(within(alert).queryByRole("button")).toBeNull();
      expect(alert.querySelector("pre")).toBeNull();
    });

    it("再生に失敗したとき見る人が変わっていれば、失敗の層を出さずに1度だけ読み直す", async () => {
      renderPage();
      await ready();
      server.session = "guest";
      act(() => player().onError(1000, "source"));
      act(() => player().onError(2000, "source"));
      await waitFor(() => expect(reloadPage).toHaveBeenCalledOnce());
      expect(screen.queryByText("Couldn't play this video")).toBeNull();
    });

    it("再生に失敗しても見る人が同じなら、今の再生失敗の層を出す", async () => {
      server.videos.set(7, [guestVideo()]);
      server.session = "guest";
      renderPage("7", undefined, "guest");
      await ready();
      act(() => player().onError(1000, "source"));
      expect(await screen.findByText("Couldn't play this video")).toBeDefined();
      expect(reloadPage).not.toHaveBeenCalled();
    });

    it("前の動画の再生失敗の確かめが別の動画へ移った後に返っても、次の動画に失敗の層を出さない", async () => {
      server.videos.set(8, [{ ...video, id: 8, title: "後続の動画" }]);
      let answerSession: (() => void) | undefined;
      const answered = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        if (String(input) !== "/api/auth/session") return answered!(input, init);
        return new Promise<Response>((resolve) => {
          answerSession = () => resolve(json({ state: "owner" }));
        });
      });
      renderPage();
      await ready();
      act(() => player().onError(1000, "source"));
      await waitFor(() => expect(answerSession).toBeDefined());
      fireEvent.click(screen.getByRole("link", { name: "別の動画" }));
      expect((await ready()).textContent).toBe("後続の動画");
      const detailFetches = () =>
        fetchMock.mock.calls.filter(([input]) => String(input) === "/api/videos/8")
          .length;
      const before = detailFetches();
      await act(async () => {
        answerSession!();
        await Promise.resolve();
      });
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(screen.queryByText("Couldn't play this video")).toBeNull();
      expect(detailFetches()).toBe(before);
      expect(reloadPage).not.toHaveBeenCalled();
    });

    it("ゲストで公開でなくなった動画は「開けません」を出す", async () => {
      server.videos.set(7, [guestVideo()]);
      server.session = "guest";
      renderPage("7", undefined, "guest");
      await ready();
      server.videos.delete(7);
      act(() => player().onError(1000, "source"));
      expect(await screen.findByText("This video can't be opened")).toBeDefined();
      expect(reloadPage).not.toHaveBeenCalled();
    });
  });

  describe("英語の画面（疑似ロケール）", () => {
    // Exclude user content and locale-independent duration, size, and technical values.
    const technical = technicalSummary(video);
    const userData = [
      "テスト動画",
      "後続の動画",
      "前の動画",
      "別の動画",
      "無効な動画",
      "旅行",
      "series",
      "ep01",
      "ep02",
      "ep03",
      formatDuration(video.durationMs),
      formatBytes(video.sizeBytes),
      ...(technical.kind === "values" ? technical.values : []),
    ];

    function end() {
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: false,
          userActive: true,
          ended: true,
          stalled: false,
          positioned: true,
        }),
      );
    }

    it("途切れの警告の文と × の読み上げ名がカタログから出る", async () => {
      enablePseudoLocale();
      renderPage();
      await ready();
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: true,
          userActive: true,
          ended: false,
          stalled: true,
          positioned: true,
        }),
      );
      expect(screen.getByRole("button", { name: "⟦Dismiss⟧" })).toBeDefined();
      expectCatalogTextOnly(document.body, userData);
    });

    it("通常の画面（帯・プレイヤーの操作・題名・タグ・情報・関連動画）の文言がカタログから出る", async () => {
      enablePseudoLocale();
      server.videos.set(7, [
        {
          ...video,
          tags: [
            {
              kind: null,
              id: 1,
              name: "旅行",
              manual: true,
              fromFolder: false,
              tentative: false,
            },
          ],
        },
      ]);
      const user = userEvent.setup();
      renderPage("7", "/?q=abc");
      await ready();
      await screen.findByRole("heading", { level: 2, name: /Related videos/ });
      await screen.findByTestId("video-player");
      expect(screen.getByRole("switch").getAttribute("aria-label")).toBe(
        "⟦Show to people who aren't signed in⟧",
      );
      expectCatalogTextOnly(document.body, userData);

      await user.type(screen.getByRole("combobox", { name: /Add tag/ }), "新");
      await screen.findByRole("option", { name: /Create/ });
      expectCatalogTextOnly(document.body, [...userData, "新"]);
    });

    it("関連動画が 0 件の画面とゲストの画面の文言がカタログから出る", async () => {
      enablePseudoLocale();
      server.related.set(7, { items: [] });
      const owner = renderPage();
      await ready();
      await waitFor(() =>
        expect(fetchMock).toHaveBeenCalledWith(
          "/api/videos/7/related",
          expect.anything(),
        ),
      );
      expectCatalogTextOnly(document.body, userData);
      owner.unmount();

      server.videos.set(7, [{ ...video, location: undefined, public: true }]);
      renderPage("7", undefined, "guest");
      await ready();
      expectCatalogTextOnly(document.body, userData);
    });

    it("処理中（段階表示と作成中の 1 行）の文言がカタログから出る", async () => {
      enablePseudoLocale();
      server.videos.set(7, [
        {
          ...video,
          probeState: "pending",
          thumbnailState: "pending",
          previewState: "pending",
          seekThumbnailState: "pending",
        },
      ]);
      const stages = renderPage();
      await screen.findByText(/Getting ready to play/);
      expectCatalogTextOnly(document.body, userData);
      stages.unmount();

      server.videos.set(7, [
        { ...video, thumbnailState: "pending", seekThumbnailState: "pending" },
      ]);
      renderPage();
      await screen.findByText(/Creating/);
      expectCatalogTextOnly(document.body, userData);
    });

    it("失敗（読み取り・再生・読み込み・無い・再生できない・開けない）の文言がカタログから出る", async () => {
      enablePseudoLocale();
      server.videos.set(7, [
        {
          ...video,
          probeState: "failed",
          probeError: "moov atom not found",
          probeErrorCode: "probe_failed",
        },
      ]);
      server.open.mockReturnValue(json({ code: "file_missing", message: "x" }, 409));
      const readFailure = renderPage();
      const alert = await screen.findByRole("alert");
      fireEvent.click(within(alert).getByRole("button", { name: /Open file/ }));
      await within(alert).findByText(/Couldn't open the file/);
      expectCatalogTextOnly(document.body, userData);
      readFailure.unmount();

      server.videos.set(7, [video]);
      const playback = renderPage();
      await ready();
      act(() => player().onError(42_000, "source"));
      await screen.findByRole("alert");
      expectCatalogTextOnly(document.body, [...userData, formatDuration(42_000)]);
      playback.unmount();

      server.videos.set(7, () => json({ code: "internal", message: "x" }, 500));
      const loadFailed = renderPage();
      await screen.findByRole("alert");
      expectCatalogTextOnly(document.body, userData);
      loadFailed.unmount();

      server.videos.delete(7);
      const missing = renderPage();
      await screen.findByRole("alert");
      expectCatalogTextOnly(document.body, userData);
      missing.unmount();

      server.videos.set(7, [{ ...video, durationMs: undefined }]);
      renderPage();
      await screen.findByRole("alert");
      expectCatalogTextOnly(document.body, userData);
    });

    it("再生終了・グループの予告・グループ操作のメニューの文言がカタログから出る", async () => {
      enablePseudoLocale();
      const ended = renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: /Related videos/ });
      end();
      await screen.findByRole("button", { name: /Play next/ });
      expectCatalogTextOnly(document.body, userData);
      ended.unmount();

      const folder = { rootId: 1, path: "series" };
      const member = (id: number, title: string, position: number): Video => ({
        ...video,
        id,
        title,
        group: { id: 1, folder, name: "series", position, count: 3 },
      });
      const members = [
        member(11, "ep01", 1),
        member(12, "ep02", 2),
        member(13, "ep03", 3),
      ];
      const group = {
        id: 1,
        name: "series",
        items: members.map((value) => ({ ...value, group: undefined })),
        offset: 0,
        total: members.length,
      };
      server.videos.set(12, [members[1]!]);
      server.related.set(12, { items: [], nextId: 13, prevId: 11, group });
      const user = userEvent.setup();
      renderPage("12", "/?q=series");
      await ready();
      await screen.findByRole("heading", { level: 2, name: /Up next/ });
      expectCatalogTextOnly(document.body, userData);

      await user.click(screen.getByRole("button", { name: /Group actions/ }));
      await screen.findByRole("menuitem", { name: /Dissolve/ });
      expectCatalogTextOnly(document.body, userData);
      await user.keyboard("{Escape}");

      end();
      await screen.findByRole("button", { name: /Play now/ });
      expectCatalogTextOnly(document.body, userData);
    });
  });
});
