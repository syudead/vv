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
import { saveListSnapshot, takeListSnapshot } from "../api/listSnapshot";
import { type Audience, AudienceProvider } from "../auth/audience";
import { reloadPage } from "../auth/pageNavigation";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { formatBytes, formatDuration } from "../lib/format";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
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
    // プレイヤーは作ったときの値だけを使う。
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
  title: "テスト動画",
  public: false,
  sizeBytes: 84_331_821,
  addedAt: "2026-09-01T00:00:00Z",
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

/** server は経路ごとの応答を差し替えられる偽のサーバーである。 */
const server = {
  videos: new Map<number, (() => Response) | Video[]>(),
  related: new Map<number, RelatedVideos>(),
  /** GET /api/videos/{id}/subtitles の応答。無い動画は字幕無し。 */
  subtitles: new Map<number, () => Response>(),
  probe: vi.fn<() => Response>(),
  open: vi.fn<() => Response>(),
  /** フォルダのまとめ方の経路（PUT …/grouping・POST …/grouping/tag）の応答。 */
  grouping: vi.fn<(method: string, url: string, body: unknown) => Response>(),
  /** PUT /api/videos/{id}/display-name の応答。 */
  displayName: vi.fn<(body: unknown) => Response>(),
  /** PUT /api/videos/{id}/thumbnail-position の応答。 */
  thumbnailPosition: vi.fn<(body: unknown) => Response | Promise<Response>>(),
  /** GET /api/auth/session が答える見る人の状態。 */
  session: "owner" as AuthState,
};

function fakeControls(): PlayerControls {
  return {
    togglePlay: vi.fn(),
    play: vi.fn(),
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

function renderPage(id = "7", from?: string, audience: Audience = "owner") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter
          initialEntries={[
            {
              pathname: `/videos/${id}`,
              state: from === undefined ? undefined : { from },
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
    server.displayName.mockReset();
    server.thumbnailPosition.mockReset();
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
      if (url.startsWith("/api/folders/")) {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
        return Promise.resolve(server.grouping(method, url, body));
      }
      if (url === "/api/tags") return Promise.resolve(json([]));
      const match = /^\/api\/videos\/(\d+)(\/[a-z-]+)?$/.exec(url);
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
      if (suffix === "/display-name" && method === "PUT") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
        return Promise.resolve(server.displayName(body));
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
      // 配列は取得のたびに先頭から 1 つずつ返し、最後の 1 つを返し続ける。
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
    it("題名・ファイルの情報・技術情報・関連動画を出し、廃止した項目を出さない", async () => {
      renderPage("7", "/?q=abc");
      expect((await ready()).textContent).toBe("テスト動画");
      // 題名は描画後の effect で入るので、h1 が出た直後ではなく反映を待つ。
      await waitFor(() => expect(document.title).toBe("テスト動画 · VVMDM"));
      expect(screen.getByRole("list", { name: "File details" })).toBeDefined();
      expect(screen.getByText("H.264")).toBeDefined();
      expect(screen.getByRole("button", { name: "Open file" })).toBeDefined();
      expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
      expect(
        await screen.findByRole("heading", { level: 2, name: "Related videos" }),
      ).toBeDefined();

      expect(screen.queryByRole("tab")).toBeNull();
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
      // × は見出しの帯に 1 つだけ置く。
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

    it("直接開いたら × は / へ、外部 URL は戻り先にしない", async () => {
      renderPage("7", "//evil.example");
      await ready();
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /");
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
      await screen.findByRole("button", { name: "Play" });
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

    it("開く要求が file_missing なら情報の行の下に出し、画面の他の部分は変えない", async () => {
      server.open.mockReturnValue(json({ code: "file_missing", message: "無い" }, 409));
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "Related videos" });
      const snapshot = () => ({
        header: document.querySelector("header")?.outerHTML,
        frame: document.querySelector("[data-player-frame]")?.outerHTML,
        title: document.querySelector("h1")?.outerHTML,
        facts: screen.getByRole("list", { name: "File details" }).outerHTML,
        technical: screen.getByRole("list", { name: "Technical details" }).outerHTML,
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
      // 情報の行のすぐ下（技術情報の上）に出る。
      const row = screen.getByRole("list", { name: "File details" }).parentElement;
      expect(row?.nextElementSibling).toBe(alert);
      // 足されるのはその 1 行だけで、帯・プレイヤー・題名・情報・関連動画は変わらない。
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

    it("プレイヤーの入れ物は列を 1 本（minmax(0,1fr)）に固定し、層を幅の中で折り返させる", async () => {
      renderPage();
      await ready();
      // 列の指定が無いと、層の列が内容の幅まで広がり、狭い幅で右が切れる。
      const frame = document.querySelector("[data-player-frame]");
      expect(frame?.className.split(" ")).toContain("grid-cols-[minmax(0,1fr)]");
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
      // タグの並びの見出し（視覚的に隠した h2「タグ」）は関連動画と関係なく常に出る。
      expect(
        screen.queryByRole("heading", { level: 2, name: "Related videos" }),
      ).toBeNull();
      expect(closeButton()).toBeDefined();
    });
  });

  describe("プレイヤーの操作", () => {
    it("タッチ用の中央は再生/一時停止だけで、秒数送りのボタンを出さない", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      fireEvent.click(await screen.findByRole("button", { name: "Play" }));
      expect(controls.togglePlay).toHaveBeenCalledTimes(1);
      expect(screen.queryByRole("button", { name: /秒戻る|秒進む/ })).toBeNull();
    });

    it("左右の端の矢印で、戻り先付きで同じフォルダの前後へ移る", async () => {
      server.videos.set(3, [{ ...related(3, "前の動画"), location: video.location }]);
      renderPage("7", "/folders/1/movies");
      await ready();
      const previous = await screen.findByRole("button", {
        name: "Previous video: 前の動画",
      });
      expect(
        screen.getByRole("button", { name: "Next video: 後続の動画" }),
      ).toBeDefined();
      // 止まっている間に移ったときは、移った先で再生を始めない。
      fireEvent.click(previous);
      await waitFor(() => expect(player().video.id).toBe(3));
      expect(player().autoplay).toBe(false);
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
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

    it("前後の矢印は操作バーと同じ時期に見せ、前後が無い側は出さない", async () => {
      server.related.set(7, { items: [related(3, "前の動画")], prevId: 3 });
      renderPage();
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

    it("中央の再生/一時停止はタップで残るフォーカスがあっても再生中の無操作で隠す", async () => {
      renderPage();
      await ready();
      const toggle = await screen.findByRole("button", { name: "Play" });
      // タップした後のようにフォーカスが残った状態にする。
      act(() => toggle.focus());
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
      const layer = document.querySelector<HTMLElement>("[data-touch-controls]");
      expect(layer?.className).toContain("opacity-0");
      // キーボードの輪郭のときだけ見せ、ただのフォーカスでは見せない。
      expect(layer?.className).not.toContain("focus-within:opacity-100");
      expect(layer?.className).toContain("has-[button:focus-visible]:opacity-100");
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

    it("関連動画の並びに無い前の動画も、題名無しで移れる", async () => {
      server.related.set(7, { items: [related(8, "後続の動画")], nextId: 8, prevId: 99 });
      renderPage();
      await ready();
      expect(await screen.findByRole("button", { name: "Previous video" })).toBeDefined();
    });

    it("画面のどこでも Space・0 がプレイヤーに効く", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "Play" });
      // 画面全体のキー操作がプレイヤーの操作を受け取るのは描画後の effect なので、0 が効く
      // ようになるのを待ってから Space を確かめる。
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
      expect(screen.getByText("Reading technical details…")).toBeDefined();

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
      // thumbnailState などが変わっても、プレイヤーは作り直さない。
      expect(playerMock.mounts).toBe(1);

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(screen.queryByText(/Creating/)).toBeNull();
      const calls = fetchMock.mock.calls.filter(
        ([input]) => String(input) === "/api/videos/7",
      );
      expect(calls).toHaveLength(4);
      await advance(10_000);
      // failed だけが残ったら取り直さない。
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
      // ffprobe の出力（probeError）は出さず、コードから英語の説明を作る。
      expect(
        within(alert).getByText("The file is damaged or isn't a supported video."),
      ).toBeDefined();
      expect(within(alert).queryByText(/moov atom/)).toBeNull();
      expect(within(alert).getByRole("button", { name: "Open file" })).toBeDefined();
      expect(screen.getByText("Couldn't read the technical details")).toBeDefined();
      // 読み取りの失敗は終わりとして扱い、プレビューが pending でも取り直さない。
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
      // 操作はプレイヤーが onControls を返してから出るので、出るまで待つ。
      (await screen.findByRole("button", { name: "Play" })).focus();
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
      group: { folder, name: "series", position, count: 3 },
    });
    const ep01 = member(11, "ep01", 1);
    const ep02 = member(12, "ep02", 2);
    const ep03 = member(13, "ep03", 3);
    const listed = (value: Video): Video => ({ ...value, group: undefined });
    const group = { folder, name: "series", items: [ep01, ep02, ep03].map(listed) };

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

    it("題名の上にグループ名と何本目かの1行を出す。所有者ではまとめ方のメニューの引き金にする", async () => {
      await openMember();
      const title = screen.getByRole("heading", { level: 1 });
      // 所有者の題名は編集ボタンと 1 行に並び、ファイル名の行とまとまりを作る
      // （specs/029-video-overrides/ui-design.md「File name line」）。その前の行である。
      const line = title.parentElement?.parentElement?.previousElementSibling;
      expect(line?.textContent).toBe("series·2 / 3");
      expect(line?.tagName).toBe("BUTTON");
      expect(line?.getAttribute("aria-label")).toBe(
        'Group "series", video 2 of 3. Grouping menu',
      );
      expect(within(line as HTMLElement).queryByRole("link")).toBeNull();
      expect(screen.getByTitle("series")).toBeDefined();
    });

    it("表示名を保存すると、グループのメンバーの並びにあるこの動画の題名も置き換わる", async () => {
      const user = userEvent.setup();
      server.displayName.mockReturnValueOnce(
        json({ ...ep02, title: "第二話", fileTitle: "ep02", displayName: "第二話" }),
      );
      await openMember();
      const members = () => document.getElementById("group-heading")!.closest("section")!;
      expect(within(members()).getByText("ep02")).toBeDefined();

      await user.click(screen.getByRole("button", { name: "Edit name" }));
      const input = screen.getByRole("textbox", { name: "Display name" });
      await user.clear(input);
      await user.type(input, "第二話{Enter}");

      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("第二話"),
      );
      expect(within(members()).getByText("第二話")).toBeDefined();
      expect(within(members()).queryByText("ep02")).toBeNull();
    });

    it("ゲストでは Group line を押せない文字の行にする", async () => {
      renderPage("12", "/", "guest");
      const title = await ready();
      const line = title.previousElementSibling;
      expect(line?.textContent).toBe("series·2 / 3");
      expect(line?.tagName).toBe("P");
      expect(screen.queryByRole("button", { name: /Grouping menu/ })).toBeNull();
    });

    describe("Group line のメニュー（ui-design.md「Group line」）", () => {
      const single = (value: Video): Video => ({ ...value, group: undefined });
      const lineButton = () =>
        screen.findByRole("button", {
          name: 'Group "series", video 2 of 3. Grouping menu',
        });

      /** ungroupOnServer は、操作の後の取り直しでグループでなくなった応答を返させる。 */
      function ungroupOnServer() {
        server.videos.set(12, [single(ep02)]);
        server.related.set(12, { items: [related(8, "後続の動画")], nextId: 8 });
      }

      it("「まとめを解除」で PUT を送り、トーストで伝え、動画と関連動画を取り直してふつうの形に戻す（受け入れ条件 3）", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ mode: "ungroup", grouped: false, taggable: false }),
        );
        await openMember();
        saveListSnapshot(
          { query: "series" },
          { items: [], total: 0, hasMore: false, scrollY: 0 },
        );
        ungroupOnServer();
        await user.click(await lineButton());
        const menu = await screen.findByRole("menu");
        expect(
          within(menu)
            .getAllByRole("menuitem")
            .map((item) => item.textContent),
        ).toEqual(["Ungroup", "Turn the group into a tag"]);
        await user.click(within(menu).getByRole("menuitem", { name: "Ungroup" }));

        expect(await screen.findByText('Ungrouped "series"')).toBeDefined();
        expect(server.grouping).toHaveBeenCalledWith(
          "PUT",
          "/api/folders/1/grouping?path=series",
          { mode: "ungroup" },
        );
        await waitFor(() =>
          expect(
            screen.getByRole("heading", { level: 1 }).previousElementSibling,
          ).toBeNull(),
        );
        await waitFor(() =>
          expect(screen.queryByRole("heading", { level: 2, name: "Up next" })).toBeNull(),
        );
        // 閉じてライブラリを開くと、控えではなく読み直した一覧が出る。
        expect(takeListSnapshot({ query: "series" })).toBeUndefined();
      });

      it("「グループをタグに変える」で POST を送り、既存のタグなら「付け」と伝える（受け入れ条件 5）", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({
            tag: { id: 4, name: "series" },
            created: false,
            grouping: { mode: "ungroup", grouped: false, taggable: false },
          }),
        );
        await openMember();
        ungroupOnServer();
        await user.click(await lineButton());
        await user.click(
          await screen.findByRole("menuitem", { name: "Turn the group into a tag" }),
        );
        expect(
          await screen.findByText('Added the tag "series" and ungrouped'),
        ).toBeDefined();
        expect(server.grouping).toHaveBeenCalledWith(
          "POST",
          "/api/folders/1/grouping/tag?path=series",
          undefined,
        );
        await waitFor(() =>
          expect(
            screen.getByRole("heading", { level: 1 }).previousElementSibling,
          ).toBeNull(),
        );
      });

      it("タグ化の失敗をトーストで伝え、画面を変えない", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ code: "invalid_request", message: "x" }, 400),
        );
        await openMember();
        await user.click(await lineButton());
        await user.click(
          await screen.findByRole("menuitem", { name: "Turn the group into a tag" }),
        );
        expect(
          await screen.findByText(
            "\"series\" can't be used as a tag name, so it can't become a tag",
          ),
        ).toBeDefined();
        const button = await lineButton();
        await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
        expect(screen.getByRole("heading", { level: 2, name: "Up next" })).toBeDefined();
      });

      it("409 では「もうグループではありません」と伝え、動画と関連動画を取り直す", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ code: "conflict", message: "x" }, 409),
        );
        await openMember();
        saveListSnapshot(
          { query: "series" },
          { items: [], total: 0, hasMore: false, scrollY: 0 },
        );
        const before = videoRequests(12);
        ungroupOnServer();
        await user.click(await lineButton());
        await user.click(await screen.findByRole("menuitem", { name: "Ungroup" }));
        expect(await screen.findByText("This folder is no longer a group")).toBeDefined();
        // ほかのタブで先に変わったので、ライブラリの控えも古い。次に開くときは読み直させる。
        expect(takeListSnapshot({ query: "series" })).toBeUndefined();
        await waitFor(() => expect(videoRequests(12)).toBe(before + 1));
        await waitFor(() =>
          expect(screen.queryByRole("heading", { level: 2, name: "Up next" })).toBeNull(),
        );
      });

      it("404 やほかの失敗は「変更できませんでした」", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ code: "not_found", message: "x" }, 404),
        );
        await openMember();
        await user.click(await lineButton());
        await user.click(await screen.findByRole("menuitem", { name: "Ungroup" }));
        expect(await screen.findByText("Couldn't make the change")).toBeDefined();
      });

      it("登録フォルダそのもののグループでは「グループをタグに変える」を出さない", async () => {
        const user = userEvent.setup();
        const rootFolder = { rootId: 1, path: "" };
        server.videos.set(12, [
          {
            ...ep02,
            group: { folder: rootFolder, name: "movies", position: 2, count: 3 },
          },
        ]);
        await openMember();
        await user.click(
          await screen.findByRole("button", {
            name: 'Group "movies", video 2 of 3. Grouping menu',
          }),
        );
        const menu = await screen.findByRole("menu");
        expect(
          within(menu)
            .getAllByRole("menuitem")
            .map((item) => item.textContent),
        ).toEqual(["Ungroup"]);
      });

      it("メニューが開いている間の Esc はメニューだけを閉じ、再生画面のままフォーカスを引き金へ戻す", async () => {
        const user = userEvent.setup();
        await openMember();
        const button = await lineButton();
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
      (await screen.findByRole("button", { name: "Play" })).focus();
      end();
      const statuses = screen
        .getAllByRole("status")
        .filter((node) => node.textContent === announcement);
      expect(statuses).toHaveLength(1);
      // フォーカスがプレイヤーの中にあったので「取り消す」へ移る。
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" }));
      // DOM の順でも「取り消す」が先。
      const cancel = screen.getByRole("button", { name: "Cancel" });
      const now = screen.getByRole("button", { name: "Play now" });
      expect(
        cancel.compareDocumentPosition(now) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      // 残り秒数は読み上げさせない。
      const seconds = screen.getByText("in 5 seconds");
      expect(seconds.getAttribute("aria-hidden")).toBe("true");
      // タッチ用の中央操作は出さない。
      expect(screen.queryByRole("button", { name: "Play" })).toBeNull();
      expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();

      await advance(2000);
      expect(screen.getByText("in 3 seconds")).toBeDefined();
      // 秒数が減っても読み上げの文は増えない。
      expect(
        screen.getAllByRole("status").filter((node) => node.textContent === announcement),
      ).toHaveLength(1);
      const before = videoRequests(13);
      await advance(3000);
      await waitFor(() => expect(player().video.id).toBe(13));
      expect(videoRequests(13)).toBeGreaterThan(before);
      expect(player().autoplay).toBe(true);
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep03");
      // メンバーを移っても × は開く前の一覧へ戻る。
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=series");
    });

    it("隠れたタブでタイマーが間引かれても、期限を過ぎていれば見えたときに次のメンバーを再生する", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      await screen.findByRole("button", { name: "Play" });
      end();
      expect(screen.getByText("in 5 seconds")).toBeDefined();
      // タイマーが一度も呼ばれないまま 30 秒経ったことにする。
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
      (await screen.findByRole("button", { name: "Play" })).focus();
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
      await screen.findByRole("button", { name: "Play" });
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
      await screen.findByRole("button", { name: "Play" });
      end();
      fireEvent.click(screen.getByRole("button", { name: "Play now" }));
      await waitFor(() => expect(player().video.id).toBe(13));
      expect(player().autoplay).toBe(true);
    });

    it("最後のメンバーでは予告を出さず、「もう一度見る」だけの層を出す（受け入れ条件 16）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember("13");
      await screen.findByRole("button", { name: "Play" });
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
      await screen.findByRole("button", { name: "Play" });
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
      await screen.findByRole("button", { name: "Play" });
      end();
      server.videos.delete(13);
      // ほかの動画の知らせでは確かめない。
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
      await screen.findByRole("button", { name: "Play" });
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
      await screen.findByRole("button", { name: "Play" });
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
      await screen.findByRole("button", { name: "Play" });
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
      // 入れ物は下の操作へ通し、× だけが押せる。状態表示の入れ物とは別の層にある。
      const layer = status.closest("[data-stall-warning]");
      expect(layer?.className).toContain("pointer-events-none");
      expect(layer?.className).toContain("left-2");
      expect(layer?.closest("[data-overlay-layer]")).toBeNull();
      expect(within(status).getByRole("button", { name: "Dismiss" }).className).toContain(
        "pointer-events-auto",
      );
    });

    it("警告が出ていても、中央の操作と操作バーの操作の入口がそのまま使える", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      report();
      expect(warning()).not.toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Pause" }));
      expect(controls.togglePlay).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId("video-player")).toBeDefined();
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
  // 公開の切り替え（specs/016-single-account-auth/ui-design.md「Visibility toggle」、issue 305）。
  describe("公開の切り替え", () => {
    /** visibility は PUT /api/video-visibility の応答を1つずつ返す。 */
    function holdVisibility() {
      const answers: ((response: Response) => void)[] = [];
      const bodies: unknown[] = [];
      const answered = fetchMock.getMockImplementation();
      fetchMock.mockImplementation((input, init) => {
        if (String(input) !== "/api/video-visibility") return answered!(input, init);
        bodies.push(JSON.parse(String(init?.body)));
        return new Promise<Response>((resolve) => answers.push(resolve));
      });
      return { answers, bodies };
    }

    function toggle() {
      return screen.getByRole("switch", { name: "Show to people who aren't signed in" });
    }

    it("所有者には非公開の状態で出し、押すと1回だけ送って応答の後に公開中へ変わる", async () => {
      const { answers, bodies } = holdVisibility();
      renderPage();
      await ready();
      expect(toggle().getAttribute("aria-checked")).toBe("false");
      expect(toggle().textContent).toBe("Private");

      fireEvent.click(toggle());
      // 送信中は押せない印（aria-disabled）で、フォーカスは外さず、応答が来るまで
      // 状態を変えない。
      expect(toggle().getAttribute("aria-disabled")).toBe("true");
      expect(toggle().getAttribute("aria-checked")).toBe("false");
      fireEvent.click(toggle());
      expect(bodies).toEqual([{ videoIds: [7], public: true }]);

      await act(async () => answers[0]!(json({ applied: 1 })));
      await waitFor(() => expect(toggle().getAttribute("aria-checked")).toBe("true"));
      expect(toggle().textContent).toBe("Public");
      expect(toggle().getAttribute("aria-disabled")).toBeNull();
      // トーストは出さない。
      expect(screen.queryByRole("status")).toBeNull();
    });

    it("公開中を押すと public: false を送り、非公開に戻る", async () => {
      server.videos.set(7, [{ ...video, public: true }]);
      const { answers, bodies } = holdVisibility();
      renderPage();
      await ready();
      expect(toggle().textContent).toBe("Public");
      fireEvent.click(toggle());
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
      // 切り替えは1つずつ順に送るので、決着させないと後のテストの要求が待たされる。
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

  describe("表示名（specs/029-video-overrides/ui-design.md「Title editing」）", () => {
    const fileTitle = "テスト動画";
    const named: Video = {
      ...video,
      title: "夏の旅行",
      fileTitle,
      displayName: "夏の旅行",
    };

    it("動画ページから移動せずに表示名を保存すると題名が置き換わり、元のファイル名が従の行で見え、解除で戻る（受け入れ条件 1・2・7）", async () => {
      const user = userEvent.setup();
      server.videos.set(7, [{ ...video, fileTitle }]);
      server.displayName
        .mockReturnValueOnce(json(named))
        .mockReturnValueOnce(json({ ...video, fileTitle }));
      renderPage();
      await ready();
      expect(screen.queryByTitle(`File name: ${fileTitle}`)).toBeNull();

      await user.click(screen.getByRole("button", { name: "Edit name" }));
      const input = screen.getByRole<HTMLInputElement>("textbox", {
        name: "Display name",
      });
      await user.clear(input);
      await user.type(input, "夏の旅行{Enter}");

      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("夏の旅行"),
      );
      expect(server.displayName).toHaveBeenCalledWith({ displayName: "夏の旅行" });
      expect(screen.getByTitle(`File name: ${fileTitle}`).textContent).toBe(
        `File name ${fileTitle}`,
      );
      expect(document.title).toBe("夏の旅行 · VVMDM");
      // 画面は移っていない。
      expect(screen.queryByTestId("screen")).toBeNull();

      await user.click(screen.getByRole("button", { name: "Edit name" }));
      await user.clear(screen.getByRole("textbox", { name: "Display name" }));
      await user.keyboard("{Enter}");
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(fileTitle),
      );
      expect(server.displayName).toHaveBeenLastCalledWith({ displayName: "" });
      expect(screen.queryByTitle(`File name: ${fileTitle}`)).toBeNull();
    });

    it("入力中は再生画面のキー操作が効かず、入力の中の Esc は編集の取り消しで画面を閉じない", async () => {
      const user = userEvent.setup();
      renderPage("7", "/?q=abc");
      await ready();
      await user.click(screen.getByRole("button", { name: "Edit name" }));
      await user.type(
        screen.getByRole("textbox", { name: "Display name" }),
        " fm0{Escape}",
      );
      const controls = playerMock.controls!;
      expect(controls.togglePlay).not.toHaveBeenCalled();
      expect(controls.toggleFullscreen).not.toHaveBeenCalled();
      expect(controls.toggleMute).not.toHaveBeenCalled();
      expect(screen.queryByTestId("screen")).toBeNull();
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("テスト動画");
      expect(server.displayName).not.toHaveBeenCalled();
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

  describe("代表サムネイル（specs/029-video-overrides/ui-design.md「Thumbnail fact」）", () => {
    const captureName = "Use current frame as thumbnail";
    const clearName = "Use automatic thumbnail";

    function report(overrides: Partial<PlayerStatus> = {}) {
      act(() =>
        player().onStatus({
          loading: false,
          reconnecting: false,
          playing: false,
          userActive: true,
          ended: false,
          stalled: false,
          positioned: true,
          ...overrides,
        }),
      );
    }

    function captureButton() {
      return screen.getByRole("button", { name: captureName });
    }

    it("最初の読み込みで位置が確定するまでは押せず、一時停止中の位置をミリ秒で送り、応答の動画で項目が出る（受け入れ条件 5）", async () => {
      const controls = fakeControls();
      vi.mocked(controls.positionMs).mockReturnValue(83_456.4);
      playerMock.controls = controls;
      server.thumbnailPosition.mockReturnValueOnce(
        json({
          ...video,
          thumbnailPositionMs: 83_456,
          thumbnailUrl: "/api/videos/7/thumbnail?v=2",
        }),
      );
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "Play" });
      expect(captureButton().getAttribute("aria-disabled")).toBe("true");
      fireEvent.click(captureButton());
      expect(server.thumbnailPosition).not.toHaveBeenCalled();
      // 位置を指定していない動画には項目が無い。
      expect(screen.queryByRole("button", { name: clearName })).toBeNull();

      report();
      expect(captureButton().getAttribute("aria-disabled")).toBeNull();
      fireEvent.click(captureButton());

      const label = `Thumbnail at ${formatDuration(83_456)}`;
      const image = await screen.findByRole("img", { name: label });
      expect(image.getAttribute("src")).toBe("/api/videos/7/thumbnail?v=2");
      expect(server.thumbnailPosition).toHaveBeenCalledTimes(1);
      expect(server.thumbnailPosition).toHaveBeenCalledWith({ positionMs: 83_456 });
      // 再生は止めない。
      expect(controls.togglePlay).not.toHaveBeenCalled();
      const facts = screen.getByRole("list", { name: "File details" });
      expect(within(facts).getByTitle(label)).toBeDefined();
      expect(within(facts).getByRole("button", { name: clearName })).toBeDefined();
      // 時刻を数値で入力させる入口は無い。
      expect(screen.queryByRole("spinbutton")).toBeNull();
      expect(screen.queryByRole("textbox")).toBeNull();
    });

    it("送信中は回転の印で二重に送らず、失敗の理由を 1 行出して前の項目を残す（Edge Case「生成に失敗」）", async () => {
      const controls = fakeControls();
      vi.mocked(controls.positionMs).mockReturnValue(5_000);
      playerMock.controls = controls;
      const chosen: Video = { ...video, thumbnailPositionMs: 60_000 };
      server.videos.set(7, [chosen]);
      let answer: (response: Response) => void = () => undefined;
      server.thumbnailPosition.mockReturnValueOnce(
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
      );
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "Play" });
      report({ playing: true });

      fireEvent.click(captureButton());
      expect(captureButton().getAttribute("aria-disabled")).toBe("true");
      fireEvent.click(captureButton());
      expect(server.thumbnailPosition).toHaveBeenCalledTimes(1);

      await act(async () => {
        answer(json({ code: "conflict", reason: "thumbnail_frame_unavailable" }, 409));
        await Promise.resolve();
      });
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe(
        "Couldn't change the thumbnail: No image could be made from this frame.",
      );
      expect(screen.getByTitle(`Thumbnail at ${formatDuration(60_000)}`)).toBeDefined();
      expect(captureButton().getAttribute("aria-disabled")).toBeNull();
    });

    it("× で解除を送り、応答で項目が消える（受け入れ条件 6）", async () => {
      server.videos.set(7, [{ ...video, thumbnailPositionMs: 60_000 }]);
      server.thumbnailPosition.mockReturnValueOnce(json(video));
      renderPage();
      await ready();
      fireEvent.click(await screen.findByRole("button", { name: clearName }));
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: clearName })).toBeNull(),
      );
      expect(server.thumbnailPosition).toHaveBeenCalledWith({ positionMs: null });
      expect(screen.queryByTitle(/^Thumbnail at/)).toBeNull();
    });

    it("再生終了の層・状態の層が映像を覆っている間は押せない", async () => {
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "Play" });
      report({ ended: true });
      expect(captureButton().getAttribute("aria-disabled")).toBe("true");
      report();
      expect(captureButton().getAttribute("aria-disabled")).toBeNull();
      act(() => player().onError(1_000, "decode"));
      await waitFor(() =>
        expect(captureButton().getAttribute("aria-disabled")).toBe("true"),
      );
    });

    it("ゲストにはボタンも項目も無い（受け入れ条件 9）", async () => {
      server.session = "guest";
      server.videos.set(7, [{ ...video, location: undefined, public: true }]);
      renderPage("7", undefined, "guest");
      await ready();
      expect(screen.queryByRole("button", { name: captureName })).toBeNull();
      expect(screen.queryByRole("button", { name: clearName })).toBeNull();
    });
  });

  describe("見る人", () => {
    function guestVideo(overrides: Partial<Video> = {}): Video {
      // ゲストの応答には location・progress・probeError が無く、tags は空である。
      return { ...video, location: undefined, tags: [], public: true, ...overrides };
    }

    it("ゲストにはタグ・ファイルの操作を出さず、再生位置を送らない", async () => {
      server.videos.set(7, [guestVideo()]);
      const { unmount } = renderPage("7", undefined, "guest");
      await ready();
      expect(screen.getByRole("list", { name: "File details" })).toBeDefined();
      expect(screen.getByRole("list", { name: "Technical details" })).toBeDefined();
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
      // サーバーはゲストに tags を空で返すが、画面の側も所有者でなければタグの
      // 並びを描かない。
      server.videos.set(7, [
        guestVideo({
          tags: [
            { id: 1, name: "高画質", manual: true, fromFolder: false, tentative: true },
            { id: 2, name: "旅行", manual: true, fromFolder: false, tentative: false },
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
    // 利用者のデータ（題名・パス・タグ名）と、ロケールに依らない値（長さ・容量・技術情報）。
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
            { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
          ],
        },
      ]);
      const user = userEvent.setup();
      renderPage("7", "/?q=abc");
      await ready();
      await screen.findByRole("heading", { level: 2, name: /Related videos/ });
      await screen.findByRole("button", { name: /Play/ });
      expect(screen.getByRole("switch").getAttribute("aria-label")).toBe(
        "⟦Show to people who aren't signed in⟧",
      );
      expectCatalogTextOnly(document.body, userData);

      // タグの入力の候補（作成の行）。
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

    it("再生終了・グループの予告・Group line のメニューの文言がカタログから出る", async () => {
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
        group: { folder, name: "series", position, count: 3 },
      });
      const members = [
        member(11, "ep01", 1),
        member(12, "ep02", 2),
        member(13, "ep03", 3),
      ];
      const group = {
        folder,
        name: "series",
        items: members.map((value) => ({ ...value, group: undefined })),
      };
      server.videos.set(12, [members[1]!]);
      server.related.set(12, { items: [], nextId: 13, prevId: 11, group });
      const user = userEvent.setup();
      renderPage("12", "/?q=series");
      await ready();
      await screen.findByRole("heading", { level: 2, name: /Up next/ });
      expectCatalogTextOnly(document.body, userData);

      await user.click(screen.getByRole("button", { name: /Grouping menu/ }));
      await screen.findByRole("menuitem", { name: /Ungroup/ });
      expectCatalogTextOnly(document.body, userData);
      await user.keyboard("{Escape}");

      end();
      await screen.findByRole("button", { name: /Play now/ });
      expectCatalogTextOnly(document.body, userData);
    });
  });
});
