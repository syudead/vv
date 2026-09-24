import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useEffect } from "react";
import { Link, MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { RelatedVideos, Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
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

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerProps | undefined,
  mounts: 0,
  controls: undefined as PlayerControls | undefined,
}));

vi.mock("./VideoPlayer", () => ({
  default: (props: PlayerProps) => {
    playerMock.props = props;
    useEffect(() => {
      playerMock.mounts += 1;
      if (playerMock.controls !== undefined) props.onControls(playerMock.controls);
      return () => props.onControls(null);
      // プレイヤーは作ったときの値だけを使う。
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
};

function fakeControls(): PlayerControls {
  return {
    togglePlay: vi.fn(),
    play: vi.fn(),
    seekBy: vi.fn(),
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

function renderPage(id = "7", from?: string) {
  return render(
    <TooltipProvider>
      <MemoryRouter
        initialEntries={[
          { pathname: `/videos/${id}`, state: from === undefined ? undefined : { from } },
        ]}
      >
        <Link to="/videos/8">別の動画</Link>
        <Link to="/videos/invalid">無効な動画</Link>
        <Routes>
          <Route path="/videos/:id" element={<VideoPage />} />
          <Route path="/" element={<Screen name="ライブラリ" />} />
          <Route path="/folders/*" element={<Screen name="フォルダ" />} />
        </Routes>
      </MemoryRouter>
    </TooltipProvider>,
  );
}

async function ready() {
  return screen.findByRole("heading", { level: 1 });
}

function closeButtons() {
  return screen.getAllByRole("button", { name: "閉じる" });
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
    });
    server.probe.mockReset();
    server.open.mockReset();
    fetchMock.mockReset();
    installFakeEventSource();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
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
    it("題名・属性の一列・場所・関連動画を出し、廃止した項目を出さない", async () => {
      renderPage("7", "/?q=abc");
      expect((await ready()).textContent).toBe("テスト動画");
      // 題名は描画後の effect で入るので、h1 が出た直後ではなく反映を待つ。
      await waitFor(() => expect(document.title).toBe("テスト動画 - vv"));
      expect(screen.getByText("RESOLUTION")).toBeDefined();
      expect(screen.getByText("H.264")).toBeDefined();
      expect(
        screen.getByRole("button", {
          name: "ファイルを開く: /media/movies/テスト動画.mp4",
        }),
      ).toBeDefined();
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
      // 幅ごとに 1 つずつ、× を 2 つ置く（見せる方は CSS で決める）。
      expect(closeButtons()).toHaveLength(2);
    });

    it("× で遷移元の一覧へ、無ければ / へ戻る", async () => {
      renderPage("7", "/folders/3/A%20B?sort=titleAsc");
      await ready();
      const [overlay] = closeButtons();
      if (overlay === undefined) throw new Error("× がありません");
      fireEvent.click(overlay);
      expect(screen.getByTestId("screen").textContent).toBe(
        "フォルダ /folders/3/A%20B?sort=titleAsc",
      );
    });

    it("直接開いたら × は / へ、外部 URL は戻り先にしない", async () => {
      renderPage("7", "//evil.example");
      await ready();
      fireEvent.click(closeButtons()[1] as HTMLElement);
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
      await screen.findByRole("button", { name: "10 秒進む" });
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(screen.queryByTestId("screen")).toBeNull();
      expect(controls.isFullscreen).toHaveBeenCalled();
    });

    it("開けない環境では場所をただの文字にする", async () => {
      server.videos.set(7, [
        { ...video, location: { path: "/media/a.mp4", openable: false } },
      ]);
      renderPage();
      await ready();
      expect(screen.queryByRole("button", { name: /ファイルを開く/ })).toBeNull();
      expect(screen.getByTitle("/media/a.mp4").tagName).toBe("P");
    });

    it("開く要求が file_missing なら場所の下に出し、画面の他の部分は変えない", async () => {
      server.open.mockReturnValue(json({ code: "file_missing", message: "無い" }, 409));
      renderPage();
      await ready();
      await screen.findByRole("heading", { level: 2, name: "関連動画" });
      const snapshot = () => ({
        frame: document.querySelector("[data-player-frame]")?.outerHTML,
        title: document.querySelector("h1")?.outerHTML,
        properties: document.querySelector("dl")?.outerHTML,
        related: document.querySelector("aside")?.outerHTML,
        location: screen.getByRole("button", { name: /ファイルを開く/ }).parentElement
          ?.outerHTML,
        text: document.body.textContent,
      });
      const before = snapshot();
      fireEvent.click(screen.getByRole("button", { name: /ファイルを開く/ }));
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe("開けませんでした: ファイルが見つかりません");
      expect(screen.getAllByRole("alert")).toHaveLength(1);
      expect(screen.getByTestId("video-player")).toBeDefined();
      expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("テスト動画");
      // 場所の行のすぐ下に出る。
      const location = screen.getByRole("button", {
        name: /ファイルを開く/,
      }).parentElement;
      expect(location?.nextElementSibling).toBe(alert);
      // 足されるのはその 1 行だけで、プレイヤー・題名・属性・場所の行・関連動画は変わらない。
      const after = snapshot();
      expect({ ...after, text: undefined }).toEqual({ ...before, text: undefined });
      const path = "/media/movies/テスト動画.mp4";
      expect(after.text).toBe(
        (before.text ?? "").replace(path, `${path}${alert.textContent ?? ""}`),
      );
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
      fireEvent.click(closeButtons()[0] as HTMLElement);
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
    });

    it("0 件のとき見出しを出さず、× は広い画面用も残す", async () => {
      server.related.set(7, { items: [] });
      renderPage();
      await ready();
      await waitFor(() => expect(screen.queryByText("関連動画")).toBeNull());
      await act(async () => undefined);
      expect(screen.queryByRole("heading", { level: 2 })).toBeNull();
      expect(closeButtons()).toHaveLength(2);
    });
  });

  describe("プレイヤーの操作", () => {
    it("タッチ用の中央の 10 秒戻る/進む で再生位置が 10 秒動く", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      fireEvent.click(await screen.findByRole("button", { name: "10 秒進む" }));
      fireEvent.click(screen.getByRole("button", { name: "10 秒戻る" }));
      fireEvent.click(screen.getByRole("button", { name: "再生" }));
      expect(controls.seekBy).toHaveBeenNthCalledWith(1, 10);
      expect(controls.seekBy).toHaveBeenNthCalledWith(2, -10);
      expect(controls.togglePlay).toHaveBeenCalledTimes(1);
    });

    it("画面のどこでも Space・→ がプレイヤーに効く", async () => {
      const controls = fakeControls();
      playerMock.controls = controls;
      renderPage();
      await ready();
      await screen.findByRole("button", { name: "10 秒進む" });
      // 画面全体のキー操作がプレイヤーの操作を受け取るのは描画後の effect なので、→ が効く
      // ようになるのを待ってから Space を確かめる。
      await waitFor(() => {
        fireEvent.keyDown(document.body, { key: "ArrowRight" });
        expect(controls.seekBy).toHaveBeenCalledWith(10);
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
      expect(screen.getAllByText("読み取り中")).toHaveLength(4);

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
      expect(screen.getByText("MEDIA")).toBeDefined();
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
      expect(closeButtons()).toHaveLength(2);
      fireEvent.click(closeButtons()[0] as HTMLElement);
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
      (await screen.findByRole("button", { name: "10 秒進む" })).focus();
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
      fireEvent.click(closeButtons()[0] as HTMLElement);
      expect(screen.getByTestId("screen").textContent).toBe("フォルダ /folders/1/movies");
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
});
