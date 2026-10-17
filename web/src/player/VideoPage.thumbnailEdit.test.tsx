import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  setRenderedAudience as forgetAudience,
  type ThumbnailEdit,
  type Video,
} from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import type { PlayerControls } from "./playerControls";
import type { PlayerStatus } from "./VideoPlayer";
import VideoPage from "./VideoPage";

// 動画ページから ComfyUI でサムネイルを編集する流れ（specs/045-comfyui-thumbnails/ui-design.md
// 「Video page: edit entry」「Thumbnail editor」「Generated thumbnail item」、
// specs/046-thumbnail-image-upload/ui-design.md「Video page: edit entry with the source choice」
// 「Thumbnail editor from the current thumbnail」）。

interface PlayerProps {
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
}

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerProps | undefined,
  controls: undefined as PlayerControls | undefined,
}));

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: PlayerProps) {
    playerMock.props = props;
    const [initial] = useState(props);
    useEffect(() => {
      if (playerMock.controls !== undefined) initial.onControls(playerMock.controls);
      return () => initial.onControls(null);
    }, [initial]);
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

const video: Video = {
  id: 7,
  title: "テスト動画",
  public: false,
  sizeBytes: 1_000,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  thumbnailUrl: "/api/videos/7/thumbnail?v=r1",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  location: { path: "/media/テスト動画.mp4", openable: false },
  tags: [],
};

const session: ThumbnailEdit = {
  id: "e1",
  videoId: 7,
  state: "ready",
  source: "frame",
  positionMs: 83_456,
  frameUrl: "/api/thumbnail-edits/e1/frame",
  frameWidth: 1920,
  frameHeight: 1080,
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** server は経路ごとの応答を差し替えられる偽のサーバーである。 */
const server = {
  ready: true,
  /** GET /api/thumbnail-edits/e1 の答え。先頭から 1 つずつ返し、最後の 1 つを返し続ける。 */
  reads: [] as ThumbnailEdit[],
  run: vi.fn<(form: FormData) => Response>(),
  apply: vi.fn<() => Response>(),
  cancel: vi.fn<() => Response>(),
  revert: vi.fn<() => Response>(),
  /** POST /api/videos/7/thumbnail-edits の答え。 */
  start: vi.fn<() => Response>(),
  /** POST /api/videos/7/thumbnail-edits で送った本文。 */
  starts: [] as unknown[],
  /** GET /api/videos/7 の答え。 */
  video,
  requests: [] as string[],
};

function controls(): PlayerControls {
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
    positionMs: vi.fn(() => 83_456.4),
  };
}

function renderPage(audience: Audience = "owner") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/videos/7"]}>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

/** positioned はプレイヤーの最初の読み込みで位置が確定したことにする（入口を押せる）。 */
function positioned() {
  act(() =>
    playerMock.props?.onStatus({
      loading: false,
      reconnecting: false,
      playing: true,
      userActive: true,
      ended: false,
      stalled: false,
      positioned: true,
    }),
  );
}

function entry() {
  return screen.getByRole("button", { name: "Edit thumbnail with ComfyUI" });
}

/** pressEntry は入口を押す（Radix のメニューの引き金は click ではなくキーで開く）。 */
function pressEntry() {
  fireEvent.keyDown(entry(), { key: "Enter" });
}

/** openMenu は入口を押して元の選択のメニューを開く。 */
async function openMenu() {
  await waitFor(() => expect(entry().getAttribute("aria-disabled")).toBeNull());
  pressEntry();
  return screen.findByRole("menu");
}

async function openPage(audience: Audience = "owner") {
  const view = renderPage(audience);
  await screen.findByRole("heading", { level: 1 });
  await screen.findByTestId("video-player");
  positioned();
  return view;
}

async function openEditor(item: string | RegExp = /^Frame at /) {
  await openPage();
  const menu = await openMenu();
  fireEvent.click(within(menu).getByRole("menuitem", { name: item }));
  return screen.findByRole("dialog", { name: "Edit thumbnail" });
}

async function tick(ms = 1000) {
  await act(() => vi.advanceTimersByTimeAsync(ms));
}

function runArea(dialog: HTMLElement) {
  return within(dialog).getByRole("status");
}

describe("VideoPage thumbnail edit", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    playerMock.props = undefined;
    playerMock.controls = controls();
    server.ready = true;
    server.reads = [session];
    server.run.mockReset();
    server.apply.mockReset();
    server.cancel.mockReset();
    server.revert.mockReset();
    server.start.mockReset();
    server.start.mockImplementation(() => json(session, 201));
    server.starts = [];
    server.video = video;
    server.requests = [];
    installFakeEventSource();
    // 前の検査で描いた見る人を忘れる。
    forgetAudience(null);
    // jsdom は canvas を描かない。塗りの層は描けないまま扱う。
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      server.requests.push(`${method} ${url}`);
      if (url === "/api/auth/session") return Promise.resolve(json({ state: "owner" }));
      if (url === "/api/thumbnail-edits/readiness")
        return Promise.resolve(json({ connection: server.ready }));
      if (url === "/api/videos/7/thumbnail-edits" && method === "POST") {
        server.starts.push(JSON.parse(String(init?.body)));
        return Promise.resolve(server.start());
      }
      if (url === "/api/thumbnail-edits/e1" && method === "GET") {
        const next = server.reads.length > 1 ? server.reads.shift() : server.reads[0];
        return Promise.resolve(json(next));
      }
      if (url === "/api/thumbnail-edits/e1" && method === "DELETE")
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url === "/api/thumbnail-edits/e1/runs")
        return Promise.resolve(server.run(init?.body as FormData));
      if (url === "/api/thumbnail-edits/e1/apply") return Promise.resolve(server.apply());
      if (url === "/api/thumbnail-edits/e1/cancel")
        return Promise.resolve(server.cancel());
      if (url === "/api/videos/7/thumbnail-image" && method === "DELETE")
        return Promise.resolve(server.revert());
      if (url === "/api/videos/7") return Promise.resolve(json(server.video));
      if (url === "/api/videos/7/related") return Promise.resolve(json({ items: [] }));
      if (url === "/api/videos/7/subtitles")
        return Promise.resolve(json({ subtitles: [] }));
      if (url === "/api/tags") return Promise.resolve(json([]));
      return Promise.resolve(json({}));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("接続が無ければ入口は薄く押せない印で、押すと読み直して Settings への案内を出す", async () => {
    server.ready = false;
    await openPage();
    await waitFor(() =>
      expect(server.requests).toContain("GET /api/thumbnail-edits/readiness"),
    );
    expect(entry().getAttribute("aria-disabled")).toBe("true");
    expect(entry().className).toContain("opacity-50");

    pressEntry();
    const link = await screen.findByRole("link", { name: "Open Settings" });
    expect(link.getAttribute("href")).toBe("/settings/connections#comfyui");
    expect(screen.getByText("Set up ComfyUI first")).toBeDefined();
    expect(
      server.requests.filter((r) => r === "GET /api/thumbnail-edits/readiness"),
    ).toHaveLength(2);
    expect(server.requests).not.toContain("POST /api/videos/7/thumbnail-edits");
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("最初の readiness の答えが押した後の読み直しより遅れて届いても、入口を薄くし直さない", async () => {
    let answerFirst: (response: Response) => void = () => undefined;
    const route = fetchMock.getMockImplementation();
    let readinessCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/thumbnail-edits/readiness" && readinessCalls++ === 0) {
        server.requests.push("GET /api/thumbnail-edits/readiness");
        return new Promise<Response>((resolve) => (answerFirst = resolve));
      }
      return route!(input, init);
    });
    await openPage();
    await waitFor(() =>
      expect(server.requests).toContain("GET /api/thumbnail-edits/readiness"),
    );
    pressEntry();
    const menu = await screen.findByRole("menu");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Frame at 1:23" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit thumbnail" });
    await act(async () => answerFirst(json({ connection: false })));

    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(entry().getAttribute("aria-disabled")).toBeNull();
  });

  it("コマの項目はメニューを開いた時点の位置を送り、今のサムネイルの項目は位置なしで送る", async () => {
    await openPage();
    let menu = await openMenu();
    const frameItem = within(menu).getByRole("menuitem", { name: "Frame at 1:23" });
    const thumbnailItem = within(menu).getByRole("menuitem", {
      name: "Current thumbnail",
    });
    expect(frameItem.getAttribute("aria-disabled")).toBeNull();
    expect(thumbnailItem.querySelector("img")?.getAttribute("src")).toBe(
      video.thumbnailUrl,
    );
    expect(thumbnailItem.querySelector("img")?.getAttribute("alt")).toBe("");
    // メニューを開いている間に再生が進んでも、項目が名指した位置を送る。
    vi.mocked(playerMock.controls!.positionMs).mockReturnValue(99_000);
    fireEvent.click(frameItem);
    let dialog = await screen.findByRole("dialog", { name: "Edit thumbnail" });
    expect(server.starts).toEqual([{ source: "frame", positionMs: 83_456 }]);
    expect(playerMock.controls?.pause).toHaveBeenCalledTimes(1);

    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    menu = await openMenu();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Current thumbnail" }));
    dialog = await screen.findByRole("dialog", { name: "Edit thumbnail" });
    expect(server.starts[1]).toEqual({ source: "thumbnail" });
    expect(playerMock.controls?.pause).toHaveBeenCalledTimes(2);
  });

  it("今のサムネイルから始めると、編集画面の詳細・ヒント・比較の名前が元を名指す", async () => {
    const saved: ThumbnailEdit = {
      ...session,
      source: "thumbnail",
      positionMs: undefined,
    };
    server.start.mockImplementation(() => json(saved, 201));
    server.reads = [saved];
    // ページのサムネイルの種類は文言に使わない。
    server.video = { ...video, thumbnailImage: "generated" };
    const dialog = await openEditor("Current thumbnail");
    expect(within(dialog).getByText("Current thumbnail, saved image")).toBeDefined();
    expect(
      within(dialog).getByText(
        "Paint over what to change. Leave it unpainted to change the whole image.",
      ),
    ).toBeDefined();
    expect(within(dialog).queryByText(/^Frame at/)).toBeNull();

    server.run.mockReturnValueOnce(
      json({ ...saved, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [
      {
        ...saved,
        state: "succeeded",
        elapsedMs: 2_000,
        resultUrl: "/api/thumbnail-edits/e1/result",
      },
    ];
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await waitFor(() =>
      expect(runArea(dialog).textContent).toContain("Waiting in the queue"),
    );
    await tick();
    const first = await within(dialog).findByRole("img", { name: "Thumbnail" });
    expect(first.getAttribute("src")).toBe("/api/thumbnail-edits/e1/frame");
    expect(within(dialog).getByText("Thumbnail")).toBeDefined();
    expect(within(dialog).queryByRole("img", { name: "Frame" })).toBeNull();
  });

  it("今のサムネイルがコマから作られていれば、詳細はその位置を名指す", async () => {
    const framed: ThumbnailEdit = { ...session, source: "thumbnail", positionMs: 61_000 };
    server.start.mockImplementation(() => json(framed, 201));
    server.reads = [framed];
    const dialog = await openEditor("Current thumbnail");
    expect(within(dialog).getByText("Current thumbnail, frame at 1:01")).toBeDefined();
  });

  it("画面にコマが無くても入口は薄くならず、コマの項目だけ押せない", async () => {
    renderPage();
    await screen.findByTestId("video-player");
    // 位置が確定していない（キャプチャのボタンは押せない）。
    await waitFor(() =>
      expect(server.requests).toContain("GET /api/thumbnail-edits/readiness"),
    );
    await waitFor(() => expect(entry().getAttribute("aria-disabled")).toBeNull());
    expect(entry().className.split(" ")).not.toContain("opacity-50");
    const menu = await openMenu();
    const frameItem = within(menu).getByRole("menuitem", { name: "Frame on screen" });
    expect(frameItem.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(frameItem);
    expect(server.starts).toEqual([]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Current thumbnail" }));
    await screen.findByRole("dialog", { name: "Edit thumbnail" });
    expect(server.starts).toEqual([{ source: "thumbnail" }]);
  });

  it("プレイヤーが出ない動画にも入口を置き、今のサムネイルから始められる", async () => {
    server.video = { ...video, probeState: "failed" };
    renderPage();
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByTestId("video-player")).toBeNull();
    const menu = await openMenu();
    expect(
      within(menu)
        .getByRole("menuitem", { name: "Frame on screen" })
        .getAttribute("aria-disabled"),
    ).toBe("true");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Current thumbnail" }));
    await screen.findByRole("dialog", { name: "Edit thumbnail" });
    expect(server.starts).toEqual([{ source: "thumbnail" }]);
  });

  it("始められなければ、情報の行の失敗の 1 行に理由を出す", async () => {
    server.start.mockImplementation(() =>
      json({ code: "internal", message: "temporarily unavailable" }, 500),
    );
    await openPage();
    const menu = await openMenu();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Current thumbnail" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Couldn't start editing the thumbnail:");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("押すと再生を止めて今の位置で始め、実行の状態を経過時間・進み具合とともに見せ、結果をコマと並べる", async () => {
    const dialog = await openEditor();
    expect(playerMock.controls?.pause).toHaveBeenCalled();
    expect(within(dialog).getByText("Frame at 1:23")).toBeDefined();
    const instructions = within(dialog).getByRole("textbox", { name: "Instructions" });
    await waitFor(() => expect(document.activeElement).toBe(instructions));

    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [
      { ...session, state: "preparing", elapsedMs: 1_000 },
      { ...session, state: "preparing", elapsedMs: 21_000 },
      { ...session, state: "running", elapsedMs: 22_000, progress: { value: 3, max: 8 } },
      {
        ...session,
        state: "succeeded",
        elapsedMs: 23_000,
        resultUrl: "/api/thumbnail-edits/e1/result",
      },
    ];
    fireEvent.change(instructions, { target: { value: "remove the caption" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));

    await waitFor(() =>
      expect(runArea(dialog).textContent).toContain("Waiting in the queue"),
    );
    expect(runArea(dialog).textContent).toContain("Elapsed 0:00");
    const form = server.run.mock.calls[0]?.[0];
    expect(form?.get("prompt")).toBe("remove the caption");
    // 何も塗っていなければマスクを送らない。
    expect(form?.has("mask")).toBe(false);
    expect(within(dialog).queryByRole("progressbar")).toBeNull();
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeDefined();

    await tick();
    await waitFor(() => expect(runArea(dialog).textContent).toContain("Preparing"));
    expect(runArea(dialog).textContent).toContain("Elapsed 0:01");
    expect(within(dialog).queryByRole("progressbar")).toBeNull();
    expect(runArea(dialog).textContent).not.toContain("The first run can take a minute");

    await tick();
    await waitFor(() =>
      expect(runArea(dialog).textContent).toContain(
        "The first run can take a minute while ComfyUI loads its models.",
      ),
    );

    await tick();
    await waitFor(() => expect(runArea(dialog).textContent).toContain("Generating"));
    expect(runArea(dialog).textContent).toContain("Elapsed 0:22");
    expect(within(dialog).getByRole("progressbar", { name: "Progress" })).toBeDefined();
    expect(runArea(dialog).textContent).toContain("Step 3 of 8");

    await tick();
    const generated = await within(dialog).findByRole("img", { name: "Generated" });
    expect(generated.getAttribute("src")).toBe("/api/thumbnail-edits/e1/result");
    expect(within(dialog).getByRole("img", { name: "Frame" }).getAttribute("src")).toBe(
      "/api/thumbnail-edits/e1/frame",
    );
    expect(
      within(dialog).getByRole("button", { name: "Use as thumbnail" }),
    ).toBeDefined();

    // 適用すると閉じ、情報の行が生成した画像の項目になる。× でコマのサムネイルへ戻る。
    server.apply.mockReturnValueOnce(
      json({
        ...video,
        thumbnailImage: "generated",
        thumbnailUrl: "/api/videos/7/thumbnail?v=g2",
      }),
    );
    server.revert.mockReturnValueOnce(json(video));
    fireEvent.click(within(dialog).getByRole("button", { name: "Use as thumbnail" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const facts = screen.getByRole("list", { name: "File details" });
    const item = within(facts).getByRole("img", { name: "Generated thumbnail" });
    expect(item.getAttribute("src")).toBe("/api/videos/7/thumbnail?v=g2");
    await waitFor(() => expect(document.activeElement).toBe(entry()));

    fireEvent.click(within(facts).getByRole("button", { name: "Use the frame instead" }));
    await waitFor(() =>
      expect(
        within(facts).queryByRole("img", { name: "Generated thumbnail" }),
      ).toBeNull(),
    );
    expect(server.revert).toHaveBeenCalledTimes(1);
    // 適用で終わったセッションは捨て直さない。
    expect(server.requests).not.toContain("DELETE /api/thumbnail-edits/e1");
  });

  it("実行が失敗したら理由の文と提供元の文を出し、入力を残して Compose に戻る", async () => {
    const dialog = await openEditor();
    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [
      {
        ...session,
        state: "failed",
        elapsedMs: 2_000,
        error: { reason: "unreachable", message: "connection refused" },
      },
    ];
    const instructions = within(dialog).getByRole("textbox", { name: "Instructions" });
    fireEvent.change(instructions, { target: { value: "make it brighter" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await waitFor(() =>
      expect(runArea(dialog).textContent).toContain("Waiting in the queue"),
    );

    await tick();
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain(
      "Couldn't generate the image: ComfyUI can't be reached",
    );
    expect(alert.textContent).toContain("connection refused");
    expect(within(dialog).getByRole("button", { name: "Generate again" })).toBeDefined();
    expect(
      (
        within(dialog).getByRole("textbox", {
          name: "Instructions",
        }) as HTMLTextAreaElement
      ).value,
    ).toBe("make it brighter");
  });

  it("実行の開始が 502 で断られたら、失敗したセッションから提供元の文を読んで出す", async () => {
    const dialog = await openEditor();
    server.run.mockReturnValueOnce(
      json(
        { code: "provider_failed", message: "provider failed", reason: "unreachable" },
        502,
      ),
    );
    server.reads = [
      {
        ...session,
        state: "failed",
        error: { reason: "unreachable", message: "dial tcp: connection refused" },
      },
    ];
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("Couldn't generate the image:");
    await waitFor(() =>
      expect(alert.textContent).toContain("dial tcp: connection refused"),
    );
  });

  it("中止が届かなくても読み続け、実行の終わりを見せる", async () => {
    const dialog = await openEditor();
    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [{ ...session, state: "running", elapsedMs: 1_000 }];
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await waitFor(() =>
      expect(runArea(dialog).textContent).toContain("Waiting in the queue"),
    );
    await tick();
    await waitFor(() => expect(runArea(dialog).textContent).toContain("Generating"));

    server.cancel.mockReturnValueOnce(
      json({ code: "internal", message: "temporarily unavailable" }, 500),
    );
    server.reads = [
      {
        ...session,
        state: "succeeded",
        elapsedMs: 3_000,
        resultUrl: "/api/thumbnail-edits/e1/result",
      },
    ];
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(server.cancel).toHaveBeenCalledTimes(1));
    await tick();
    await tick();
    expect(await within(dialog).findByRole("img", { name: "Generated" })).toBeDefined();
  });

  it("実行を中止して閉じても、結果を破棄しても、適用せずにセッションを捨て、サムネイルを変えない", async () => {
    const deletes = () =>
      server.requests.filter((r) => r === "DELETE /api/thumbnail-edits/e1").length;
    // 前の検査の編集画面がページを離れたときに捨てる要求を、先に済ませておく。
    await tick(10);
    server.requests = [];
    // 中止: 実行中のうちに中止する。
    let dialog = await openEditor();
    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [{ ...session, state: "running", elapsedMs: 1_000 }];
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await tick();
    await waitFor(() => expect(runArea(dialog).textContent).toContain("Generating"));
    server.cancel.mockReturnValueOnce(
      json({ ...session, state: "cancelled", elapsedMs: 2_000 }),
    );
    server.reads = [{ ...session, state: "cancelled", elapsedMs: 2_000 }];
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(
      await within(dialog).findByText("Cancelled. The thumbnail is unchanged."),
    ).toBeDefined();
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(deletes()).toBe(1));

    // 破棄: 結果が出てから捨てる。
    server.reads = [session];
    const menu = await openMenu();
    fireEvent.click(within(menu).getByRole("menuitem", { name: /^Frame at / }));
    dialog = await screen.findByRole("dialog", { name: "Edit thumbnail" });
    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [
      {
        ...session,
        state: "succeeded",
        elapsedMs: 1_000,
        resultUrl: "/api/thumbnail-edits/e1/result",
      },
    ];
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await tick();
    await within(dialog).findByRole("img", { name: "Generated" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(deletes()).toBe(2));

    expect(server.apply).not.toHaveBeenCalled();
    expect(screen.queryByRole("img", { name: "Generated thumbnail" })).toBeNull();
  });

  it("ページを離れるとセッションを捨てる", async () => {
    const view = renderPage();
    await screen.findByTestId("video-player");
    positioned();
    const menu = await openMenu();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Frame at 1:23" }));
    await screen.findByRole("dialog", { name: "Edit thumbnail" });

    view.unmount();
    await tick(10);
    expect(server.requests).toContain("DELETE /api/thumbnail-edits/e1");
  });

  it("実行中に閉じるときは確かめ、捨てるとセッションを捨てて入口へフォーカスを戻す", async () => {
    const dialog = await openEditor();
    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    server.reads = [{ ...session, state: "queued", elapsedMs: 1_000 }];
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await waitFor(() =>
      expect(runArea(dialog).textContent).toContain("Waiting in the queue"),
    );

    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    const confirm = await screen.findByRole("alertdialog", {
      name: "Discard this edit?",
    });
    expect(server.requests).not.toContain("DELETE /api/thumbnail-edits/e1");
    fireEvent.click(within(confirm).getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.requests).toContain("DELETE /api/thumbnail-edits/e1");
    await waitFor(() => expect(document.activeElement).toBe(entry()));
  });

  it("ゲストには入口も、ComfyUI への要求も無い", async () => {
    await openPage("guest");
    expect(
      screen.queryByRole("button", { name: "Edit thumbnail with ComfyUI" }),
    ).toBeNull();
    expect(server.requests.some((r) => r.includes("thumbnail-edits"))).toBe(false);
  });
});
