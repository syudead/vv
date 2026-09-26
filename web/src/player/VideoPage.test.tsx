import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { Link, MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AuthState } from "../api/auth";
import { type RelatedVideos, setRenderedAudience, type Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { saveListSnapshot, takeListSnapshot } from "../api/listSnapshot";
import { type Audience, AudienceProvider } from "../auth/audience";
import { reloadPage } from "../auth/pageNavigation";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import type { PlayerControls } from "./playerControls";
import type { PlayerStatus } from "./VideoPlayer";
import VideoPage from "./VideoPage";

interface PlayerProps {
  video: Video;
  initialPositionMs: number;
  autoplay: boolean;
  onPosition: (positionMs: number) => void;
  onProgress: (positionMs: number, immediate: boolean) => void;
  onError: (positionMs: number) => void;
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
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
    useEffect(() => {
      playerMock.mounts += 1;
      if (playerMock.controls !== undefined) props.onControls(playerMock.controls);
      return () => props.onControls(null);
      // プレイヤーは作ったときの値だけを使う。
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: (video: Video) =>
    video.probeState === "done" &&
    (video.durationMs ?? 0) > 0 &&
    video.videoCodec !== undefined,
  initialPlayerStatus: { loading: false, playing: false, userActive: true, ended: false },
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
  probe: vi.fn<() => Response>(),
  open: vi.fn<() => Response>(),
  /** フォルダのまとめ方の経路（PUT …/grouping・POST …/grouping/tag）の応答。 */
  grouping: vi.fn<(method: string, url: string, body: unknown) => Response>(),
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
    isFullscreen: vi.fn(() => false),
    menuOpen: vi.fn(() => false),
    wake: vi.fn(),
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
  return screen.getByRole("button", { name: "閉じる" });
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
    server.videos.set(7, [video]);
    server.related.set(7, {
      items: [related(8, "後続の動画"), related(3, "前の動画")],
      nextId: 8,
      prevId: 3,
    });
    server.probe.mockReset();
    server.open.mockReset();
    server.grouping.mockReset();
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
      const match = /^\/api\/videos\/(\d+)(\/[a-z]+)?$/.exec(url);
      if (match === null) return Promise.resolve(json({}));
      const id = Number(match[1]);
      const suffix = match[2];
      if (suffix === "/related") {
        return Promise.resolve(json(server.related.get(id) ?? { items: [] }));
      }
      if (suffix === "/probe" && method === "POST")
        return Promise.resolve(server.probe());
      if (suffix === "/open" && method === "POST") return Promise.resolve(server.open());
      if (suffix === "/progress") return Promise.resolve(json({}));
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
      await waitFor(() => expect(document.title).toBe("テスト動画 - vv"));
      expect(screen.getByRole("list", { name: "ファイルの情報" })).toBeDefined();
      expect(screen.getByText("H.264")).toBeDefined();
      expect(screen.getByRole("button", { name: "ファイルを開く" })).toBeDefined();
      expect(screen.getByRole("button", { name: "パスをコピー" })).toBeDefined();
      expect(
        await screen.findByRole("heading", { level: 2, name: "関連動画" }),
      ).toBeDefined();

      expect(screen.queryByRole("tab")).toBeNull();
      expect(screen.queryByRole("link", { name: /ライブラリ|フォルダ/ })).toBeNull();
      const text = document.body.textContent ?? "";
      for (const removed of [
        "視聴済み",
        "から再開",
        "解析",
        "ブラウザ再生",
        "バイト",
        "1080p",
      ]) {
        expect(text).not.toContain(removed);
      }
      // × は見出しの帯に 1 つだけ置く。
      expect(screen.getAllByRole("button", { name: "閉じる" })).toHaveLength(1);
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
      await screen.findByRole("button", { name: "再生" });
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
      expect(screen.queryByRole("button", { name: "ファイルを開く" })).toBeNull();
      expect(screen.getByRole("button", { name: "パスをコピー" })).toBeDefined();
    });

    it("見出しの帯に置き場所のパンくずを出し、段からそのフォルダ画面へ移る", async () => {
      server.videos.set(7, [
        { ...video, folder: { rootId: 1, path: "movies", rootName: "media" } },
      ]);
      renderPage("7", "/?q=abc");
      await ready();
      const nav = screen.getByRole("navigation", { name: "フォルダ" });
      fireEvent.click(within(nav).getByRole("link", { name: "movies" }));
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
    });

    it("ロゴからホームへ移る", async () => {
      renderPage("7", "/folders/1/movies");
      await ready();
      fireEvent.click(screen.getByRole("link", { name: "ホーム" }));
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /");
    });

    it("開く要求が file_missing なら情報の行の下に出し、画面の他の部分は変えない", async () => {
      server.open.mockReturnValue(json({ code: "file_missing", message: "無い" }, 409));
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      const snapshot = () => ({
        header: document.querySelector("header")?.outerHTML,
        frame: document.querySelector("[data-player-frame]")?.outerHTML,
        title: document.querySelector("h1")?.outerHTML,
        facts: screen.getByRole("list", { name: "ファイルの情報" }).outerHTML,
        technical: screen.getByRole("list", { name: "技術情報" }).outerHTML,
        related: document.querySelector("aside")?.outerHTML,
      });
      const before = snapshot();
      fireEvent.click(screen.getByRole("button", { name: "ファイルを開く" }));
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe("開けませんでした: ファイルが見つかりません");
      expect(screen.getAllByRole("alert")).toHaveLength(1);
      expect(screen.getByTestId("video-player")).toBeDefined();
      // 情報の行のすぐ下（技術情報の上）に出る。
      const row = screen.getByRole("list", { name: "ファイルの情報" }).parentElement;
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

  describe("関連動画", () => {
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
      await waitFor(() => expect(screen.queryByText("関連動画")).toBeNull());
      await act(async () => undefined);
      // タグの並びの見出し（視覚的に隠した h2「タグ」）は関連動画と関係なく常に出る。
      expect(screen.queryByRole("heading", { level: 2, name: "関連動画" })).toBeNull();
      expect(closeButton()).toBeDefined();
    });
  });

  describe("プレイヤーの操作", () => {
    it("タッチ用の中央は再生/一時停止だけで、秒数送りのボタンを出さない", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      fireEvent.click(await screen.findByRole("button", { name: "再生" }));
      expect(controls.togglePlay).toHaveBeenCalledTimes(1);
      expect(screen.queryByRole("button", { name: /秒戻る|秒進む/ })).toBeNull();
    });

    it("左右の端の矢印で、戻り先付きで同じフォルダの前後へ移る", async () => {
      server.videos.set(3, [{ ...related(3, "前の動画"), location: video.location }]);
      renderPage("7", "/folders/1/movies");
      await ready();
      const previous = await screen.findByRole("button", { name: "前の動画: 前の動画" });
      expect(screen.getByRole("button", { name: "次の動画: 後続の動画" })).toBeDefined();
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
      const next = await screen.findByRole("button", { name: "次の動画: 後続の動画" });
      act(() =>
        player().onStatus({
          loading: false,
          playing: true,
          userActive: true,
          ended: false,
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
      const previous = await screen.findByRole("button", { name: "前の動画: 前の動画" });
      expect(screen.queryByRole("button", { name: /^次の動画/ })).toBeNull();
      expect(previous.className).toContain("opacity-100");
      act(() =>
        player().onStatus({
          loading: false,
          playing: true,
          userActive: false,
          ended: false,
        }),
      );
      expect(previous.className).toContain("opacity-0");
      expect(previous.className).toContain("pointer-events-none");
    });

    it("全画面の間は、前後の矢印の題名の吹き出しを全画面の入れ物の中に描く", async () => {
      renderPage();
      await ready();
      const next = await screen.findByRole("button", { name: "次の動画: 後続の動画" });
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
      expect(await screen.findByRole("button", { name: "前の動画" })).toBeDefined();
    });

    it("画面のどこでも Space・0 がプレイヤーに効く", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "再生" });
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
        "ファイルの検出完了",
        "動画情報の読み取り処理中",
        "サムネイル待機中",
        "シーク用プレビュー待機中",
        "一覧用プレビュー待機中",
      ]);
      expect(screen.getByText("再生の準備をしています")).toBeDefined();
      expect(screen.queryByTestId("video-player")).toBeNull();
      expect(screen.getByText("技術情報を読み取り中")).toBeDefined();

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(screen.getByTestId("video-player")).toBeDefined();
      expect(screen.queryByText("再生の準備をしています")).toBeNull();
      expect(
        screen.getByText(
          "サムネイルとシーク用プレビューと一覧用プレビューを作成中 · 再生はできます",
        ),
      ).toBeDefined();

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(screen.getByText("一覧用プレビューを作成中 · 再生はできます")).toBeDefined();
      // thumbnailState などが変わっても、プレイヤーは作り直さない。
      expect(playerMock.mounts).toBe(1);

      await emitServerEvent("video", { id: 7 });
      await advance(0);
      expect(screen.queryByText(/を作成中/)).toBeNull();
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
        { ...pending, probeState: "failed", probeError: "moov atom not found" },
      ]);
      renderPage();
      await screen.findByText("再生の準備をしています");
      await emitServerEvent("video", { id: 7 });
      await advance(0);
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("この動画を読み取れませんでした")).toBeDefined();
      expect(within(alert).getByText("moov atom not found")).toBeDefined();
      expect(within(alert).getByRole("button", { name: "ファイルを開く" })).toBeDefined();
      expect(screen.getByText("技術情報を読み取れませんでした")).toBeDefined();
      // 読み取りの失敗は終わりとして扱い、プレビューが pending でも取り直さない。
      await advance(10_000);
      expect(
        fetchMock.mock.calls.filter(([input]) => String(input) === "/api/videos/7"),
      ).toHaveLength(2);
      expect(screen.queryByText(/を作成中/)).toBeNull();
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
      fireEvent.click(await screen.findByRole("button", { name: "もう一度読み取る" }));
      expect(await screen.findByText("再生の準備をしています")).toBeDefined();
      expect(server.probe).toHaveBeenCalledTimes(1);
      expect(screen.queryByText("読み取りを始められませんでした")).toBeNull();
    });

    it("「もう一度読み取る」が受け付けられなければ、その旨を出してボタンを戻す", async () => {
      server.videos.set(7, [
        { ...pending, probeState: "failed", probeError: "壊れています" },
      ]);
      server.probe.mockReturnValue(json({ code: "internal", message: "失敗" }, 500));
      renderPage();
      const button = await screen.findByRole("button", { name: "もう一度読み取る" });
      fireEvent.click(button);
      expect(await screen.findByText("読み取りを始められませんでした")).toBeDefined();
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
      expect(within(alert).getByText("この動画は開けません")).toBeDefined();
      expect(screen.queryByTestId("video-player")).toBeNull();
      expect(closeButton()).toBeDefined();
      fireEvent.click(closeButton());
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=a");
    });

    it("最初の取得が 404 でも「開けません」を出し、再試行は置かない", async () => {
      server.videos.delete(7);
      renderPage();
      expect(await screen.findByText("この動画は開けません")).toBeDefined();
      expect(screen.queryByRole("button", { name: "再試行" })).toBeNull();
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
      expect(within(alert).getByText("この動画を読み込めませんでした")).toBeDefined();
      expect(within(alert).getByText("データベースに届きません")).toBeDefined();
      expect(screen.queryByText("この動画は開けません")).toBeNull();
      fireEvent.click(within(alert).getByRole("button", { name: "再試行" }));
      expect((await ready()).textContent).toBe("テスト動画");
      expect(screen.getByTestId("video-player")).toBeDefined();
    });

    it("再生失敗の再試行は失敗した位置から、自動で再生を始める", async () => {
      renderPage();
      await ready();
      act(() => player().onError(42_000));
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("再生できませんでした")).toBeDefined();
      fireEvent.click(
        within(alert).getByRole("button", { name: "0:42 からもう一度試す" }),
      );
      await waitFor(() => expect(playerMock.mounts).toBe(2));
      expect(player().initialPositionMs).toBe(42_000);
      expect(player().autoplay).toBe(true);
      expect(screen.queryByText("再生できませんでした")).toBeNull();
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
          playing: false,
          userActive: true,
          ended: true,
        }),
      );
    }

    it("ended で次の動画と「次を再生」「もう一度見る」を出し、もう一度見るで先頭から再生する", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      end();
      expect(screen.getByText("再生が終わりました")).toBeDefined();
      expect(screen.getByText("次の動画")).toBeDefined();
      expect(screen.getAllByRole("link", { name: /後続の動画/ })).toHaveLength(2);
      fireEvent.click(screen.getByRole("button", { name: "もう一度見る" }));
      expect(controls.restart).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("button", { name: "次を再生" })).toBeDefined();
    });

    it("フォーカスがプレイヤーの中にあったときだけ、主な操作へフォーカスを移す", async () => {
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      // 操作はプレイヤーが onControls を返してから出るので、出るまで待つ。
      (await screen.findByRole("button", { name: "再生" })).focus();
      end();
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "次を再生" }),
      );
    });

    it("プレイヤーの外（関連動画）にいる人のフォーカスは奪わない", async () => {
      renderPage();
      await ready();
      const link = await screen.findByRole("link", { name: /後続の動画/ });
      link.focus();
      end();
      expect(screen.getByRole("button", { name: "次を再生" })).toBeDefined();
      expect(document.activeElement).toBe(link);
    });

    it("同じフォルダの後続が無ければ「次を再生」を出さない", async () => {
      server.related.set(7, { items: [related(3, "前の動画")] });
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      end();
      expect(screen.getByRole("button", { name: "もう一度見る" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "次を再生" })).toBeNull();
    });

    it("「次を再生」で戻り先付きで次の動画へ移り、移った先で再生を始める", async () => {
      server.videos.set(8, [{ ...related(8, "後続の動画"), location: video.location }]);
      renderPage("7", "/folders/1/movies");
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      end();
      fireEvent.click(screen.getByRole("button", { name: "次を再生" }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("後続の動画"),
      );
      await waitFor(() => expect(player().video.id).toBe(8));
      expect(player().autoplay).toBe(true);
      expect(screen.queryByText("再生が終わりました")).toBeNull();
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
          playing: false,
          userActive: true,
          ended: true,
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
      await screen.findByRole("heading", { level: 2, name: "続けて再生" });
    }

    const announcement = "再生が終わりました。5 秒後に次の動画「ep03」を再生します";

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
      const line = title.previousElementSibling;
      expect(line?.textContent).toBe("series·2 / 3");
      expect(line?.tagName).toBe("BUTTON");
      expect(line?.getAttribute("aria-label")).toBe(
        "グループ「series」、3 本中 2 本目。まとめ方のメニュー",
      );
      expect(within(line as HTMLElement).queryByRole("link")).toBeNull();
      expect(screen.getByTitle("series")).toBeDefined();
    });

    it("ゲストでは Group line を押せない文字の行にする", async () => {
      renderPage("12", "/", "guest");
      const title = await ready();
      const line = title.previousElementSibling;
      expect(line?.textContent).toBe("series·2 / 3");
      expect(line?.tagName).toBe("P");
      expect(screen.queryByRole("button", { name: /まとめ方のメニュー/ })).toBeNull();
    });

    describe("Group line のメニュー（ui-design.md「Group line」）", () => {
      const single = (value: Video): Video => ({ ...value, group: undefined });
      const lineButton = () =>
        screen.findByRole("button", {
          name: "グループ「series」、3 本中 2 本目。まとめ方のメニュー",
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
        ).toEqual(["まとめを解除", "グループをタグに変える"]);
        await user.click(within(menu).getByRole("menuitem", { name: "まとめを解除" }));

        expect(await screen.findByText("「series」のまとめを解除しました")).toBeDefined();
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
          expect(
            screen.queryByRole("heading", { level: 2, name: "続けて再生" }),
          ).toBeNull(),
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
          await screen.findByRole("menuitem", { name: "グループをタグに変える" }),
        );
        expect(
          await screen.findByText("タグ「series」を付け、まとめを解除しました"),
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
          await screen.findByRole("menuitem", { name: "グループをタグに変える" }),
        );
        expect(
          await screen.findByText(
            "「series」はタグの名前に使えないため、タグに変えられません",
          ),
        ).toBeDefined();
        const button = await lineButton();
        await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
        expect(
          screen.getByRole("heading", { level: 2, name: "続けて再生" }),
        ).toBeDefined();
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
        await user.click(await screen.findByRole("menuitem", { name: "まとめを解除" }));
        expect(
          await screen.findByText("このフォルダはもうグループではありません"),
        ).toBeDefined();
        // ほかのタブで先に変わったので、ライブラリの控えも古い。次に開くときは読み直させる。
        expect(takeListSnapshot({ query: "series" })).toBeUndefined();
        await waitFor(() => expect(videoRequests(12)).toBe(before + 1));
        await waitFor(() =>
          expect(
            screen.queryByRole("heading", { level: 2, name: "続けて再生" }),
          ).toBeNull(),
        );
      });

      it("404 やほかの失敗は「変更できませんでした」", async () => {
        const user = userEvent.setup();
        server.grouping.mockImplementation(() =>
          json({ code: "not_found", message: "x" }, 404),
        );
        await openMember();
        await user.click(await lineButton());
        await user.click(await screen.findByRole("menuitem", { name: "まとめを解除" }));
        expect(await screen.findByText("変更できませんでした")).toBeDefined();
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
            name: "グループ「movies」、3 本中 2 本目。まとめ方のメニュー",
          }),
        );
        const menu = await screen.findByRole("menu");
        expect(
          within(menu)
            .getAllByRole("menuitem")
            .map((item) => item.textContent),
        ).toEqual(["まとめを解除"]);
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
      const heading = screen.getByRole("heading", { level: 2, name: "続けて再生" });
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
      ).toEqual(["準備中4:02後続の動画"]);
      expect(within(others).queryByRole("link", { name: /ep0/ })).toBeNull();
    });

    it("前後のつまみはグループの中の前後で、題名をメンバーの並びから引く", async () => {
      await openMember();
      expect(screen.getByRole("button", { name: "前の動画: ep01" })).toBeDefined();
      expect(screen.getByRole("button", { name: "次の動画: ep03" })).toBeDefined();
    });

    it("再生が終わると予告を一度だけ読み上げ、5 秒後に確かめてから次のメンバーを再生する（受け入れ条件 16・19）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      (await screen.findByRole("button", { name: "再生" })).focus();
      end();
      const statuses = screen
        .getAllByRole("status")
        .filter((node) => node.textContent === announcement);
      expect(statuses).toHaveLength(1);
      // フォーカスがプレイヤーの中にあったので「取り消す」へ移る。
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "取り消す" }),
      );
      // DOM の順でも「取り消す」が先。
      const cancel = screen.getByRole("button", { name: "取り消す" });
      const now = screen.getByRole("button", { name: "今すぐ再生" });
      expect(
        cancel.compareDocumentPosition(now) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
      // 残り秒数は読み上げさせない。
      const seconds = screen.getByText("5 秒後");
      expect(seconds.getAttribute("aria-hidden")).toBe("true");
      // タッチ用の中央操作は出さない。
      expect(screen.queryByRole("button", { name: "再生" })).toBeNull();
      expect(screen.queryByRole("button", { name: "一時停止" })).toBeNull();

      await advance(2000);
      expect(screen.getByText("3 秒後")).toBeDefined();
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
      await screen.findByRole("button", { name: "再生" });
      end();
      expect(screen.getByText("5 秒後")).toBeDefined();
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
      (await screen.findByRole("button", { name: "再生" })).focus();
      end();
      fireEvent.click(screen.getByRole("button", { name: "取り消す" }));
      expect(screen.queryByText(announcement)).toBeNull();
      expect(screen.getByText("再生が終わりました")).toBeDefined();
      const playNext = screen.getByRole("button", { name: "次を再生" });
      expect(document.activeElement).toBe(playNext);
      expect(screen.getByRole("button", { name: "もう一度見る" })).toBeDefined();
      await advance(6000);
      expect(player().video.id).toBe(12);
    });

    it("予告中の Esc は取り消しで画面を閉じず、取り消した後の Esc は閉じる", async () => {
      await openMember();
      await screen.findByRole("button", { name: "再生" });
      end();
      fireEvent.keyDown(screen.getByRole("button", { name: "取り消す" }), {
        key: "Escape",
      });
      expect(screen.queryByRole("button", { name: "取り消す" })).toBeNull();
      expect(screen.getByRole("button", { name: "次を再生" })).toBeDefined();
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep02");
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(screen.getByTestId("screen").textContent).toBe("ライブラリ /?q=series");
    });

    it("「今すぐ再生」で戻り先付きで次のメンバーへ移る", async () => {
      await openMember();
      await screen.findByRole("button", { name: "再生" });
      end();
      fireEvent.click(screen.getByRole("button", { name: "今すぐ再生" }));
      await waitFor(() => expect(player().video.id).toBe(13));
      expect(player().autoplay).toBe(true);
    });

    it("最後のメンバーでは予告を出さず、「もう一度見る」だけの層を出す（受け入れ条件 16）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember("13");
      await screen.findByRole("button", { name: "再生" });
      end();
      expect(screen.queryByRole("button", { name: "取り消す" })).toBeNull();
      expect(screen.getByRole("button", { name: "もう一度見る" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "次を再生" })).toBeNull();
      await advance(6000);
      expect(player().video.id).toBe(13);
    });

    it("グループに属さない動画は、終わっても予告を出さず自動で進まない（受け入れ条件 17）", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      end();
      expect(screen.queryByRole("button", { name: "取り消す" })).toBeNull();
      expect(screen.getByRole("button", { name: "次を再生" })).toBeDefined();
      await advance(6000);
      expect(player().video.id).toBe(7);
    });

    it("予告の終わりに次のメンバーが無ければ、先へ進まず「もう一度見る」だけの層にする", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      await screen.findByRole("button", { name: "再生" });
      end();
      server.videos.delete(13);
      await advance(5000);
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: "取り消す" })).toBeNull(),
      );
      expect(screen.getByRole("button", { name: "もう一度見る" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "次を再生" })).toBeNull();
      expect(player().video.id).toBe(12);
    });

    it("予告中に次のメンバーが消えた知らせが届いたら、確かめて予告をやめる", async () => {
      await openMember();
      await screen.findByRole("button", { name: "再生" });
      end();
      server.videos.delete(13);
      // ほかの動画の知らせでは確かめない。
      const before = videoRequests(13);
      await emitServerEvent("video", { id: 8 });
      expect(videoRequests(13)).toBe(before);
      await emitServerEvent("video", { id: 13 });
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: "取り消す" })).toBeNull(),
      );
      expect(screen.getByRole("button", { name: "もう一度見る" })).toBeDefined();
      expect(screen.queryByRole("button", { name: "次を再生" })).toBeNull();
    });

    it("自動で開いたメンバーが再生に失敗したら、再生失敗の層で止まり先へ進まない", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      await openMember();
      await screen.findByRole("button", { name: "再生" });
      end();
      await advance(5000);
      await waitFor(() => expect(player().video.id).toBe(13));
      act(() => player().onError(12_000));
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("再生できませんでした")).toBeDefined();
      await advance(6000);
      expect(player().video.id).toBe(13);
      expect(screen.queryByRole("button", { name: "取り消す" })).toBeNull();
    });

    it("メンバーの並びから別のメンバーへ移っても、Esc で開く前の一覧へ戻る（受け入れ条件 19）", async () => {
      await openMember();
      fireEvent.click(screen.getByRole("link", { name: /ep01/ }));
      await waitFor(() =>
        expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("ep01"),
      );
      await screen.findByRole("heading", { level: 2, name: "続けて再生" });
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
      await screen.findByRole("heading", { level: 2, name: "続けて再生" });
      await screen.findByRole("button", { name: "再生" });
      end();
      expect(screen.getByRole("button", { name: "取り消す" })).toBeDefined();
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
      act(() => player().onError(1000));
      expect(await screen.findByText("再生できませんでした")).toBeDefined();
      fireEvent.click(screen.getByRole("link", { name: "無効な動画" }));
      expect(await screen.findByText("この動画は開けません")).toBeDefined();
      expect(screen.queryByText("再生できませんでした")).toBeNull();
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
      return screen.getByRole("switch", { name: "ログインしていない人に公開する" });
    }

    it("所有者には非公開の状態で出し、押すと1回だけ送って応答の後に公開中へ変わる", async () => {
      const { answers, bodies } = holdVisibility();
      renderPage();
      await ready();
      expect(toggle().getAttribute("aria-checked")).toBe("false");
      expect(toggle().textContent).toBe("非公開");

      fireEvent.click(toggle());
      // 送信中は押せない印（aria-disabled）で、フォーカスは外さず、応答が来るまで
      // 状態を変えない。
      expect(toggle().getAttribute("aria-disabled")).toBe("true");
      expect(toggle().getAttribute("aria-checked")).toBe("false");
      fireEvent.click(toggle());
      expect(bodies).toEqual([{ videoIds: [7], public: true }]);

      await act(async () => answers[0]!(json({ applied: 1 })));
      await waitFor(() => expect(toggle().getAttribute("aria-checked")).toBe("true"));
      expect(toggle().textContent).toBe("公開中");
      expect(toggle().getAttribute("aria-disabled")).toBeNull();
      // トーストは出さない。
      expect(screen.queryByRole("status")).toBeNull();
    });

    it("公開中を押すと public: false を送り、非公開に戻る", async () => {
      server.videos.set(7, [{ ...video, public: true }]);
      const { answers, bodies } = holdVisibility();
      renderPage();
      await ready();
      expect(toggle().textContent).toBe("公開中");
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
      expect(alert.textContent).toBe("変更できませんでした");
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

  describe("見る人", () => {
    function guestVideo(overrides: Partial<Video> = {}): Video {
      // ゲストの応答には location・progress・probeError が無く、tags は空である。
      return { ...video, location: undefined, tags: [], public: true, ...overrides };
    }

    it("ゲストにはタグ・ファイルの操作を出さず、再生位置を送らない", async () => {
      server.videos.set(7, [guestVideo()]);
      const { unmount } = renderPage("7", undefined, "guest");
      await ready();
      expect(screen.getByRole("list", { name: "ファイルの情報" })).toBeDefined();
      expect(screen.getByRole("list", { name: "技術情報" })).toBeDefined();
      expect(screen.queryByRole("heading", { name: "タグ" })).toBeNull();
      expect(screen.queryByPlaceholderText("タグを追加")).toBeNull();
      expect(screen.queryByRole("button", { name: /ファイルを開く/ })).toBeNull();
      expect(screen.queryByRole("button", { name: "パスをコピー" })).toBeNull();

      act(() => player().onProgress(30_000, true));
      act(() => player().onPosition(31_000));
      unmount();
      const progressCalls = fetchMock.mock.calls.filter(([input]) =>
        String(input).endsWith("/progress"),
      );
      expect(progressCalls).toHaveLength(0);
    });

    it("所有者にはタグの並びとファイルの操作を出す", async () => {
      renderPage();
      await ready();
      expect(screen.getByRole("heading", { name: "タグ" })).toBeDefined();
      expect(screen.getByRole("button", { name: "パスをコピー" })).toBeDefined();
    });

    it("ゲストの読み取り失敗には、やり直し・ファイルを開く・誤りの文を出さない", async () => {
      server.videos.set(7, [guestVideo({ probeState: "failed" })]);
      renderPage("7", undefined, "guest");
      const alert = await screen.findByRole("alert");
      expect(within(alert).getByText("この動画を読み取れませんでした")).toBeDefined();
      expect(within(alert).queryByRole("button")).toBeNull();
      expect(alert.querySelector("pre")).toBeNull();
    });

    it("再生に失敗したとき見る人が変わっていれば、失敗の層を出さずに1度だけ読み直す", async () => {
      renderPage();
      await ready();
      server.session = "guest";
      act(() => player().onError(1000));
      act(() => player().onError(2000));
      await waitFor(() => expect(reloadPage).toHaveBeenCalledOnce());
      expect(screen.queryByText("再生できませんでした")).toBeNull();
    });

    it("再生に失敗しても見る人が同じなら、今の再生失敗の層を出す", async () => {
      server.videos.set(7, [guestVideo()]);
      server.session = "guest";
      renderPage("7", undefined, "guest");
      await ready();
      act(() => player().onError(1000));
      expect(await screen.findByText("再生できませんでした")).toBeDefined();
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
      act(() => player().onError(1000));
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
      expect(screen.queryByText("再生できませんでした")).toBeNull();
      expect(detailFetches()).toBe(before);
      expect(reloadPage).not.toHaveBeenCalled();
    });

    it("ゲストで公開でなくなった動画は「開けません」を出す", async () => {
      server.videos.set(7, [guestVideo()]);
      server.session = "guest";
      renderPage("7", undefined, "guest");
      await ready();
      server.videos.delete(7);
      act(() => player().onError(1000));
      expect(await screen.findByText("この動画は開けません")).toBeDefined();
      expect(reloadPage).not.toHaveBeenCalled();
    });
  });
});
