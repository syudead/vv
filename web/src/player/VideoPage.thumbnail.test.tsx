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
  fileModifiedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  thumbnailUrl: "/api/videos/7/thumbnail?v=r1",
  contentVersion: "r1",
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

const uploaded: Video = {
  ...video,
  thumbnailImage: "uploaded",
  thumbnailUrl: "/api/videos/7/thumbnail?v=r1-g3",
};

/** The short-lived session that reads the current thumbnail for the preview. */
const sourceSession: ThumbnailEdit = {
  id: "s0",
  videoId: 7,
  state: "ready",
  source: "thumbnail",
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
  start: vi.fn<() => Response>(),
  starts: [] as unknown[],
  startImage: vi.fn<() => Response>(),
  upload: vi.fn<(form: FormData) => Response | Promise<Response>>(),
  position: vi.fn<(body: unknown) => Response>(),
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

/** Radix tabs select on mouse down, not on click. */
function openTab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }));
}

async function openPage(audience: Audience = "owner") {
  const view = renderPage(audience);
  await screen.findByRole("heading", { level: 1 });
  await screen.findByTestId("video-player");
  positioned();
  return view;
}

async function openThumbnail() {
  await openPage();
  openTab("Thumbnail");
}

function way(name: string) {
  return screen.getByRole("button", { name });
}

function current() {
  return screen.getByRole("region", { name: "Current" });
}

async function openPreview() {
  await openThumbnail();
  fireEvent.click(way("Touch up…"));
  return screen.findByRole("dialog", { name: "Preview thumbnail" });
}

async function openEditor() {
  const preview = await openPreview();
  fireEvent.click(
    await within(preview).findByRole("button", { name: "Paint or add a reference…" }),
  );
  return screen.findByRole("dialog", { name: "Edit thumbnail" });
}

async function uploadDialog() {
  const open = screen.queryByRole("dialog", { name: "Upload a thumbnail" });
  if (open !== null) return open;
  fireEvent.click(way("Upload…"));
  return screen.findByRole("dialog", { name: "Upload a thumbnail" });
}

async function choose(file: File) {
  const dialog = await uploadDialog();
  const input = within(dialog).getByTestId<HTMLInputElement>("image-upload-input");
  fireEvent.change(input, { target: { files: [file] } });
}

function png(name = "photo.png") {
  return new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], name, {
    type: "image/png",
  });
}

async function applyUpload() {
  await choose(png());
  const preview = await screen.findByRole("dialog", { name: "Preview thumbnail" });
  fireEvent.click(within(preview).getByRole("button", { name: "Use as thumbnail" }));
}

function posts() {
  return server.requests.filter((r) => r === "POST /api/videos/7/thumbnail-image");
}

async function tick(ms = 1000) {
  await act(() => vi.advanceTimersByTimeAsync(ms));
}

function runArea(dialog: HTMLElement) {
  return within(dialog).getByRole("status");
}

describe("VideoPage Thumbnail tab", () => {
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
    server.start.mockReset();
    server.start.mockImplementation(() => json(sourceSession, 201));
    server.starts = [];
    server.startImage.mockReset();
    server.startImage.mockImplementation(() => json(session, 201));
    server.upload.mockReset();
    server.position.mockReset();
    compose.loadUploadImage.mockReset();
    compose.loadUploadImage.mockResolvedValue({
      image: { bitmap: { close: vi.fn() }, width: 1920, height: 1080 },
    });
    compose.encodeComposition.mockReset();
    compose.encodeComposition.mockResolvedValue(
      new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], { type: "image/png" }),
    );
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
      if (url === "/api/work-info-plugins") return Promise.resolve(json({ plugins: [] }));
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
      if (url === "/api/videos/7/thumbnail-image" && method === "POST")
        return Promise.resolve(server.upload(init?.body as FormData));
      if (url === "/api/videos/7/thumbnail-position" && method === "PUT")
        return Promise.resolve(server.position(JSON.parse(String(init?.body))));
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

  it("位置と適用画像の両方がある動画の「Back to automatic」は位置 null の 1 回の要求で戻し、画像だけを外す要求は送らず、自動になると消える", async () => {
    server.video = { ...uploaded, thumbnailPositionMs: 12_000 };
    server.position.mockReturnValueOnce(
      json({ ...video, thumbnailUrl: "/api/videos/7/thumbnail?v=r2" }),
    );
    await openThumbnail();
    expect(within(current()).getByText("Uploaded image")).toBeDefined();

    fireEvent.click(within(current()).getByRole("button", { name: "Back to automatic" }));
    expect(await within(current()).findByText("Automatic")).toBeDefined();
    expect(server.position.mock.calls).toEqual([[{ positionMs: null }]]);
    expect(server.requests).not.toContain("DELETE /api/videos/7/thumbnail-image");
    expect(within(current()).getByRole("img").getAttribute("src")).toBe(
      "/api/videos/7/thumbnail?v=r2",
    );
    expect(
      within(current()).queryByRole("button", { name: "Back to automatic" }),
    ).toBeNull();
  });

  it("「Use this frame」はプレイヤーの位置を送り、Current が応答のサムネイルと位置の出どころに変わる（受け入れ条件 5）", async () => {
    server.position.mockReturnValueOnce(
      json({
        ...video,
        thumbnailPositionMs: 83_456,
        thumbnailUrl: "/api/videos/7/thumbnail?v=r1-p",
      }),
    );
    await openThumbnail();
    expect(within(current()).getByText("Automatic")).toBeDefined();
    // The row names the way and notes the player's position it will send.
    const row = within(way("Use this frame").closest("li") as HTMLElement);
    expect(row.getByText("The frame on screen")).toBeDefined();
    expect(row.getByText("1:23")).toBeDefined();

    fireEvent.click(way("Use this frame"));
    expect(await within(current()).findByText("Frame at 1:23")).toBeDefined();
    expect(server.position.mock.calls).toEqual([[{ positionMs: 83_456 }]]);
    expect(within(current()).getByRole("img").getAttribute("src")).toBe(
      "/api/videos/7/thumbnail?v=r1-p",
    );
  });

  it("プレイヤーが出ない動画では「Use this frame」が無く、「Upload…」と「Touch up with ComfyUI」は残る", async () => {
    server.video = { ...video, probeState: "failed" };
    renderPage();
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByTestId("video-player")).toBeNull();
    openTab("Thumbnail");
    expect(screen.queryByRole("button", { name: "Use this frame" })).toBeNull();
    expect(way("Upload…")).toBeDefined();
    expect(way("Touch up…")).toBeDefined();
  });

  it("「Upload…」は PNG・JPEG・WebP に限ったアップロードのダイアログを開き、選んだファイルで送らずにプレビューを開く", async () => {
    await openThumbnail();
    const dialog = await uploadDialog();
    const input = within(dialog).getByTestId<HTMLInputElement>("image-upload-input");
    expect(input.accept).toBe("image/png,image/jpeg,image/webp");

    await choose(png());
    const preview = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    expect(within(preview).getByText("photo.png")).toBeDefined();
    expect(screen.queryByRole("dialog", { name: "Upload a thumbnail" })).toBeNull();
    expect(posts()).toHaveLength(0);
  });

  it("アップロードのダイアログに貼り付けた画像でもプレビューを開く", async () => {
    await openThumbnail();
    fireEvent.paste(await uploadDialog(), {
      clipboardData: { types: ["Files"], files: [png("pasted.png")] },
    });
    const preview = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    expect(within(preview).getByText("pasted.png")).toBeDefined();
    expect(posts()).toHaveLength(0);
  });

  it("別の種類や 64 MB を超える画像は、読まずにアップロードのダイアログの中で断る", async () => {
    await openThumbnail();
    await choose(new File(["GIF89a"], "anim.gif", { type: "image/gif" }));
    const dialog = await uploadDialog();
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "Choose a PNG, JPEG or WebP image.",
    );
    const large = png("large.png");
    Object.defineProperty(large, "size", { value: 64 * 1024 * 1024 + 1 });
    await choose(large);
    await waitFor(() =>
      expect(within(dialog).getByRole("alert").textContent).toBe(
        "Choose an image up to 64 MB.",
      ),
    );
    expect(compose.loadUploadImage).not.toHaveBeenCalled();
  });

  it("「Touch up with ComfyUI」は今のサムネイルを位置なしで読んでプレビューを開き、読んだセッションは捨てる", async () => {
    const preview = await openPreview();
    expect(server.starts).toEqual([{ source: "thumbnail" }]);
    expect(within(preview).getByText("Current thumbnail")).toBeDefined();
    expect(server.requests).toContain("GET /api/thumbnail-edits/s0/frame");
    await waitFor(() =>
      expect(server.requests).toContain("DELETE /api/thumbnail-edits/s0"),
    );
  });

  it("ComfyUI が無ければ、プレビューは仕上げの欄の代わりに設定への案内を出す", async () => {
    server.ready = false;
    const preview = await openPreview();
    expect(
      (await within(preview).findByRole("link", { name: "Open Settings" })).getAttribute(
        "href",
      ),
    ).toBe("/settings/connections#comfyui");
    expect(within(preview).queryByRole("button", { name: "Remove text" })).toBeNull();
  });

  it("今のサムネイルを読み始められなければ、パネルの失敗の行に理由を出す", async () => {
    server.start.mockImplementation(() =>
      json({ code: "internal", message: "temporarily unavailable" }, 500),
    );
    await openThumbnail();
    fireEvent.click(way("Touch up…"));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Couldn't start editing the thumbnail:");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("送信中はほかの入口が aria-disabled で何も送らず、Overview へ移っても送信は終わり、戻ると結果が出る", async () => {
    let answer: (response: Response) => void = () => undefined;
    server.upload.mockReturnValueOnce(
      new Promise((resolve) => {
        answer = resolve;
      }),
    );
    await openThumbnail();
    await applyUpload();
    await waitFor(() => expect(way("Upload…").getAttribute("aria-busy")).toBe("true"));
    const form = server.upload.mock.calls[0]?.[0];
    expect((form?.get("image") as File).type).toBe("image/png");
    expect(form?.get("contentVersion")).toBe("r1");
    for (const name of ["Use this frame", "Touch up…"]) {
      expect(way(name).getAttribute("aria-disabled")).toBe("true");
      fireEvent.click(way(name));
    }
    expect(server.position).not.toHaveBeenCalled();
    expect(server.starts).toEqual([]);

    openTab("Overview");
    expect(screen.queryByRole("region", { name: "Current" })).toBeNull();
    answer(json(uploaded));
    await tick(0);

    openTab("Thumbnail");
    expect(await within(current()).findByText("Uploaded image")).toBeDefined();
    expect(within(current()).getByRole("img").getAttribute("src")).toBe(
      "/api/videos/7/thumbnail?v=r1-g3",
    );
    expect(way("Touch up…").getAttribute("aria-disabled")).toBeNull();
    expect(posts()).toHaveLength(1);
  });

  it("content_changed で断られたら失敗の行を出し、動画を取り直す", async () => {
    server.upload.mockReturnValueOnce(
      json({ code: "conflict", message: "changed", reason: "content_changed" }, 409),
    );
    await openThumbnail();
    const reads = () => server.requests.filter((r) => r === "GET /api/videos/7").length;
    const before = reads();
    await applyUpload();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't upload the image: The video changed in the meantime.",
    );
    await waitFor(() => expect(reads()).toBeGreaterThan(before));
  });

  describe("シークプレビューのスプライト", () => {
    const spriteUrl = "/api/videos/7/seek-thumbnails/sprite?v=1";
    const withSprite: Video = {
      ...video,
      seekThumbnailUrl: spriteUrl,
      seekThumbnailState: "done",
    };
    let spriteResponse: () => Response;

    beforeEach(() => {
      // jsdom has no object URLs; each loaded sheet is shown through one.
      vi.stubGlobal(
        "URL",
        class extends URL {
          static createObjectURL = () => "blob:sheet";
          static revokeObjectURL = () => undefined;
        },
      );
      spriteResponse = () =>
        json({
          intervalMs: 10_000,
          frameCount: 3,
          columns: 2,
          rows: 1,
          frameWidth: 160,
          frameHeight: 90,
          sheets: ["/sheets/0", "/sheets/1"],
        });
      const answered = fetchMock.getMockImplementation()!;
      fetchMock.mockImplementation((input, init) => {
        const url = String(input);
        if (url === spriteUrl) {
          server.requests.push(`GET ${url}`);
          return Promise.resolve(spriteResponse());
        }
        if (url.startsWith("/sheets/")) {
          server.requests.push(`GET ${url}`);
          return Promise.resolve(new Response("sheet"));
        }
        return answered(input, init);
      });
    });

    it("スプライトのある動画はシートを画像として出すだけで、コマから位置は送らない（フィードバック 3）", async () => {
      server.video = withSprite;
      await openThumbnail();
      const list = await screen.findByRole("list", { name: "Seek preview" });
      await waitFor(() =>
        expect(server.requests).toEqual(
          expect.arrayContaining(["GET /sheets/0", "GET /sheets/1"]),
        ),
      );
      expect(
        (await within(list).findAllByRole("img")).map((image) =>
          image.getAttribute("alt"),
        ),
      ).toEqual(["Seek preview sheet 1 of 2", "Seek preview sheet 2 of 2"]);
      expect(within(list).queryByRole("button")).toBeNull();
      expect(server.position).not.toHaveBeenCalled();
    });

    it("スプライトを読めなければ理由と Retry を出し、Retry で読み直す", async () => {
      server.video = withSprite;
      const ok = spriteResponse;
      spriteResponse = () => json({ code: "internal", message: "x" }, 500);
      await openThumbnail();
      expect(await screen.findByText("Couldn't load the seek preview")).toBeDefined();
      const reads = () => server.requests.filter((r) => r === `GET ${spriteUrl}`).length;
      const before = reads();
      spriteResponse = ok;
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(await screen.findByRole("list", { name: "Seek preview" })).toBeDefined();
      expect(reads()).toBe(before + 1);
    });

    it("スプライトの無い動画と作成中の動画にはシートを出さず、スプライトも読まない", async () => {
      server.video = { ...withSprite, seekThumbnailState: "pending" };
      await openThumbnail();
      expect(way("Upload…")).toBeDefined();
      expect(screen.queryByRole("list", { name: "Seek preview" })).toBeNull();
      cleanup();

      server.video = video;
      server.requests = [];
      await openThumbnail();
      expect(screen.queryByRole("list", { name: "Seek preview" })).toBeNull();
      expect(server.requests).not.toContain(`GET ${spriteUrl}`);
    });
  });

  it("ゲストには Thumbnail タブも、ComfyUI への要求も無い", async () => {
    await openPage("guest");
    expect(screen.queryByRole("tab", { name: "Thumbnail" })).toBeNull();
    expect(server.requests.some((r) => r.includes("thumbnail-edits"))).toBe(false);
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

  it("「Add image…」のダイアログにドロップした画像は参照画像になる", async () => {
    // jsdom has no object URLs; the editor shows the reference through one.
    vi.stubGlobal(
      "URL",
      class extends URL {
        static createObjectURL = () => "blob:ref";
        static revokeObjectURL = () => undefined;
      },
    );
    const dialog = await openEditor();
    fireEvent.click(within(dialog).getByRole("button", { name: "Add image…" }));
    const upload = await screen.findByRole("dialog", { name: "Add a reference image" });
    const reference = new File(["ref"], "reference.png", { type: "image/png" });
    fireEvent.drop(upload, {
      dataTransfer: { types: ["Files"], files: [reference] },
    });
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Add a reference image" })).toBeNull(),
    );
    expect(within(dialog).getByText("reference.png")).toBeTruthy();
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
});
