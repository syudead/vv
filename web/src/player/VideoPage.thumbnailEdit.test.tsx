import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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
import { PRESETS } from "./thumbnailEdit/presets";
import VideoPage from "./VideoPage";

interface PlayerProps {
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
}

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerProps | undefined,
  controls: undefined as PlayerControls | undefined,
}));

// jsdom lacks canvas rendering and image decoding; replace those boundaries.
const compose = vi.hoisted(() => ({
  loadUploadImage: vi.fn(),
  drawComposition: vi.fn(),
  drawOverflow: vi.fn(),
  encodeComposition: vi.fn(),
}));

vi.mock("./thumbnailUpload/composeImage", () => compose);

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

/** The short-lived session that reads a frame or the current thumbnail for the preview. */
const sourceSession: ThumbnailEdit = {
  id: "s0",
  videoId: 7,
  state: "ready",
  source: "frame",
  positionMs: 83_456,
  frameUrl: "/api/thumbnail-edits/s0/frame",
  frameWidth: 1920,
  frameHeight: 1080,
};

/** The editor's session, started from the preview's framing. */
const session: ThumbnailEdit = {
  id: "e1",
  videoId: 7,
  state: "ready",
  source: "image",
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

const server = {
  ready: true,
  /** Consume responses in order, then repeat the last response. */
  reads: [] as ThumbnailEdit[],
  run: vi.fn<(form: FormData) => Response>(),
  apply: vi.fn<() => Response>(),
  cancel: vi.fn<() => Response>(),
  revert: vi.fn<() => Response>(),
  start: vi.fn<() => Response>(),
  starts: [] as unknown[],
  startImage: vi.fn<() => Response>(),
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

/** Open Radix triggers with the keyboard rather than click. */
async function openMenu() {
  fireEvent.keyDown(await screen.findByRole("button", { name: "Change thumbnail" }), {
    key: "Enter",
  });
  return screen.findByRole("menu");
}

async function openPage(audience: Audience = "owner") {
  const view = renderPage(audience);
  await screen.findByRole("heading", { level: 1 });
  await screen.findByTestId("video-player");
  positioned();
  return view;
}

async function openPreview(item: string | RegExp = "Current thumbnail") {
  const menu = await openMenu();
  fireEvent.click(within(menu).getByRole("menuitem", { name: item }));
  return screen.findByRole("dialog", { name: "Preview thumbnail" });
}

async function openEditor() {
  await openPage();
  const preview = await openPreview();
  fireEvent.click(
    await within(preview).findByRole("button", { name: "Paint or add a reference…" }),
  );
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
    server.start.mockImplementation(() => json(sourceSession, 201));
    server.starts = [];
    server.startImage.mockReset();
    server.startImage.mockImplementation(() => json(session, 201));
    compose.loadUploadImage.mockReset();
    compose.loadUploadImage.mockResolvedValue({
      image: { bitmap: { close: vi.fn() }, width: 1920, height: 1080 },
    });
    compose.encodeComposition.mockReset();
    compose.encodeComposition.mockResolvedValue(new Blob(["framing"]));
    server.video = video;
    server.requests = [];
    installFakeEventSource();
    forgetAudience(null);
    // jsdom cannot draw the canvas; keep the mask layer unavailable.
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
      if (url === "/api/videos/7/thumbnail-edits/image" && method === "POST")
        return Promise.resolve(server.startImage());
      if (url === "/api/thumbnail-edits/s0/frame")
        return Promise.resolve(new Response("frame"));
      if (url === "/api/thumbnail-edits/s0" && method === "DELETE")
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url === "/api/thumbnail-edits/e1/result")
        return Promise.resolve(new Response("result"));
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

  it("ComfyUI が無くても画面のコマから始められ、開いた時点の位置で読み、仕上げの欄の代わりに設定への案内を出す", async () => {
    server.ready = false;
    await openPage();
    const menu = await openMenu();
    vi.mocked(playerMock.controls!.positionMs).mockReturnValue(99_000);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Frame at 1:23" }));
    const preview = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    expect(server.starts).toEqual([{ source: "frame", positionMs: 83_456 }]);
    expect(playerMock.controls?.pause).toHaveBeenCalledTimes(1);
    expect(within(preview).getByText("Frame at 1:23")).toBeDefined();
    expect(server.requests).toContain("GET /api/thumbnail-edits/s0/frame");
    await waitFor(() =>
      expect(server.requests).toContain("DELETE /api/thumbnail-edits/s0"),
    );
    expect(
      (await within(preview).findByRole("link", { name: "Open Settings" })).getAttribute(
        "href",
      ),
    ).toBe("/settings/connections#comfyui");
    expect(within(preview).queryByRole("button", { name: "Remove text" })).toBeNull();
  });

  it("今のサムネイルの項目は絵を添えて並び、位置なしで読む", async () => {
    await openPage();
    const menu = await openMenu();
    const item = within(menu).getByRole("menuitem", { name: "Current thumbnail" });
    expect(item.querySelector("img")?.getAttribute("src")).toBe(video.thumbnailUrl);
    expect(item.querySelector("img")?.getAttribute("alt")).toBe("");
    expect(
      within(menu).getByRole("menuitem", { name: "Upload an image…" }),
    ).toBeDefined();
    fireEvent.click(item);
    const preview = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    expect(server.starts).toEqual([{ source: "thumbnail" }]);
    expect(within(preview).getByText("Current thumbnail")).toBeDefined();
  });

  it("プレイヤーが出ない動画では、コマの項目だけ押せない", async () => {
    server.video = { ...video, probeState: "failed" };
    renderPage();
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByTestId("video-player")).toBeNull();
    const menu = await openMenu();
    const frameItem = within(menu).getByRole("menuitem", { name: "Frame on screen" });
    expect(frameItem.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(frameItem);
    expect(server.starts).toEqual([]);
  });

  it("読み始められなければ、情報の行の失敗の 1 行に理由を出す", async () => {
    server.start.mockImplementation(() =>
      json({ code: "internal", message: "temporarily unavailable" }, 500),
    );
    await openPage();
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Current thumbnail" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Couldn't start editing the thumbnail:");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("プレビューの構図から編集を始め、実行の状態を経過時間・進み具合とともに見せ、結果を構図と並べ、使うとプレビューに戻す", async () => {
    const dialog = await openEditor();
    expect(server.startImage).toHaveBeenCalledTimes(1);
    expect(within(dialog).getByText("Your framing")).toBeDefined();
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
    expect(within(dialog).getByRole("img", { name: "Framing" }).getAttribute("src")).toBe(
      "/api/thumbnail-edits/e1/frame",
    );

    compose.loadUploadImage.mockClear();
    fireEvent.click(within(dialog).getByRole("button", { name: "Use result" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Edit thumbnail" })).toBeNull(),
    );
    expect(server.requests).toContain("GET /api/thumbnail-edits/e1/result");
    expect(server.requests).toContain("DELETE /api/thumbnail-edits/e1");
    expect(compose.loadUploadImage).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("dialog", { name: "Preview thumbnail" })).toBeDefined();
    expect(server.apply).not.toHaveBeenCalled();
  });

  it("定型を押すと指示欄にその文が入り、その指示で実行する", async () => {
    const dialog = await openEditor();
    server.run.mockReturnValueOnce(
      json({ ...session, state: "queued", elapsedMs: 0 }, 202),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove text" }));
    const instructions = within(dialog).getByRole("textbox", { name: "Instructions" });
    expect((instructions as HTMLTextAreaElement).value).toBe(PRESETS[2]?.prompt);
    fireEvent.click(within(dialog).getByRole("button", { name: "Generate" }));
    await waitFor(() => expect(server.run).toHaveBeenCalled());
    expect(server.run.mock.calls[0]?.[0].get("prompt")).toBe(PRESETS[2]?.prompt);
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

  it("ページを離れるとセッションを捨てる", async () => {
    await openEditor();
    cleanup();
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
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Edit thumbnail" })).toBeNull(),
    );
    expect(server.requests).toContain("DELETE /api/thumbnail-edits/e1");
    expect(screen.getByRole("dialog", { name: "Preview thumbnail" })).toBeDefined();
  });

  it("ゲストには入口も、ComfyUI への要求も無い", async () => {
    await openPage("guest");
    expect(screen.queryByRole("button", { name: "Change thumbnail" })).toBeNull();
    expect(server.requests.some((r) => r.includes("thumbnail-edits"))).toBe(false);
  });
});
