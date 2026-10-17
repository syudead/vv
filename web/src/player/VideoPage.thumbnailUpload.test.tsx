import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { setRenderedAudience as forgetAudience, type Video } from "../api/client";
import { installFakeEventSource } from "../api/fakeEventSource";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import type { PlayerControls } from "./playerControls";
import type { PlayerStatus } from "./VideoPlayer";
import VideoPage from "./VideoPage";

// 動画ページから手元の画像をプレビューで確かめてサムネイルにする流れ
//

interface PlayerProps {
  onControls: (controls: PlayerControls | null) => void;
  onStatus: (status: PlayerStatus) => void;
}

const playerMock = vi.hoisted(() => ({
  props: undefined as PlayerProps | undefined,
}));

// jsdom には canvas も画像の復号も無いので、読む・描く・PNG にするところを差し替える。
const compose = vi.hoisted(() => ({
  loadUploadImage: vi.fn(),
  drawComposition: vi.fn(),
  encodeComposition: vi.fn(),
}));

vi.mock("./thumbnailUpload/composeImage", () => compose);

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: PlayerProps) {
    playerMock.props = props;
    const [initial] = useState(props);
    useEffect(() => {
      initial.onControls({
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
        positionMs: vi.fn(() => 12_000),
      });
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

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** server は経路ごとの応答を差し替えられる偽のサーバーである。 */
const server = {
  /** GET /api/videos/7 の答え。 */
  video,
  upload: vi.fn<(form: FormData) => Response | Promise<Response>>(),
  revert: vi.fn<() => Response>(),
  requests: [] as string[],
};

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

async function openPage(audience: Audience = "owner") {
  renderPage(audience);
  await screen.findByRole("heading", { level: 1 });
  await screen.findByTestId("video-player");
  act(() =>
    playerMock.props?.onStatus({
      loading: false,
      reconnecting: false,
      playing: false,
      userActive: true,
      ended: false,
      stalled: false,
      positioned: true,
    }),
  );
}

function entry() {
  return screen.getByRole("button", { name: "Upload an image as thumbnail" });
}

function choose(file: File) {
  const input = screen.getByTestId<HTMLInputElement>("thumbnail-upload-input");
  fireEvent.change(input, { target: { files: [file] } });
}

function png(name = "photo.png") {
  return new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], name, {
    type: "image/png",
  });
}

/** chooseAndApply は画像を選び、開いたプレビューでそのまま適用する。 */
async function chooseAndApply(file = png()) {
  choose(file);
  const dialog = await screen.findByRole("dialog", { name: "Preview thumbnail" });
  fireEvent.click(within(dialog).getByRole("button", { name: "Use as thumbnail" }));
}

const bitmap = { close: vi.fn() };

function facts() {
  return screen.getByRole("list", { name: "File details" });
}

function posts() {
  return server.requests.filter((r) => r === "POST /api/videos/7/thumbnail-image");
}

describe("VideoPage thumbnail upload", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    playerMock.props = undefined;
    server.video = video;
    server.upload.mockReset();
    server.revert.mockReset();
    server.requests = [];
    bitmap.close.mockReset();
    compose.loadUploadImage.mockReset();
    compose.loadUploadImage.mockResolvedValue({
      image: { bitmap, width: 1000, height: 2000 },
    });
    compose.encodeComposition.mockReset();
    compose.encodeComposition.mockResolvedValue(
      new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], { type: "image/png" }),
    );
    installFakeEventSource();
    forgetAudience(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      server.requests.push(`${method} ${url}`);
      if (url === "/api/auth/session") return Promise.resolve(json({ state: "owner" }));
      if (url === "/api/thumbnail-edits/readiness")
        return Promise.resolve(json({ connection: true }));
      if (url === "/api/videos/7/thumbnail-image" && method === "POST")
        return Promise.resolve(server.upload(init?.body as FormData));
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
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("ゲストには入口も、アップロードした画像の項目も無い", async () => {
    server.video = uploaded;
    await openPage("guest");
    expect(
      screen.queryByRole("button", { name: "Upload an image as thumbnail" }),
    ).toBeNull();
    expect(screen.queryByTestId("thumbnail-upload-input")).toBeNull();
    expect(within(facts()).queryByRole("img", { name: "Uploaded thumbnail" })).toBeNull();
  });

  it("入口は編集の入口の直後に置き、PNG・JPEG・WebP に限ったピッカーを開く", async () => {
    await openPage();
    const buttons = screen
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));
    const edit = buttons.indexOf("Edit thumbnail with ComfyUI");
    expect(buttons[edit + 1]).toBe("Upload an image as thumbnail");
    const input = screen.getByTestId<HTMLInputElement>("thumbnail-upload-input");
    expect(input.accept).toBe("image/png,image/jpeg,image/webp");
    const click = vi.spyOn(input, "click");
    fireEvent.click(entry());
    expect(click).toHaveBeenCalledTimes(1);
  });

  it("別の種類や 64 MB を超える画像は、要求を送らずに失敗の行で断る", async () => {
    await openPage();
    choose(new File(["GIF89a"], "anim.gif", { type: "image/gif" }));
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Choose a PNG, JPEG or WebP image.",
    );

    const large = png("large.png");
    Object.defineProperty(large, "size", { value: 64 * 1024 * 1024 + 1 });
    choose(large);
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("Choose an image up to 64 MB."),
    );
    expect(posts()).toHaveLength(0);
    expect(server.upload).not.toHaveBeenCalled();
  });

  it("画像を選ぶとすぐには送らず、全体を収めたプレビューを開き、やめると何も送らない", async () => {
    await openPage();
    choose(png());
    const dialog = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    expect(within(dialog).getByText("photo.png")).toBeDefined();
    const whole = within(dialog).getByRole("radio", { name: "Whole image" });
    expect(whole.getAttribute("aria-checked")).toBe("true");
    expect(within(dialog).queryByRole("slider", { name: "Zoom" })).toBeNull();
    expect(compose.drawComposition).toHaveBeenLastCalledWith(
      expect.any(HTMLCanvasElement),
      expect.objectContaining({ width: 1000, height: 2000 }),
      16 / 9,
      { mode: "contain" },
    );

    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Preview thumbnail" })).toBeNull(),
    );
    expect(posts()).toHaveLength(0);
    expect(bitmap.close).toHaveBeenCalled();
    await waitFor(() => expect(entry().getAttribute("aria-disabled")).toBeNull());
  });

  it("読めない画像はプレビューを出さずに、理由の文で失敗の行を出す", async () => {
    compose.loadUploadImage.mockResolvedValueOnce({
      refusal: "thumbnail_upload_too_large",
    });
    await openPage();
    choose(png());
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't upload the image: The image is too large: up to 64 MB and 120 megapixels.",
    );
    expect(screen.queryByRole("dialog", { name: "Preview thumbnail" })).toBeNull();
    expect(posts()).toHaveLength(0);
  });

  it("枠いっぱいに広げて拡大してから適用すると、その形で描いた画像を送る", async () => {
    server.upload.mockReturnValueOnce(json(uploaded));
    await openPage();
    choose(png());
    const dialog = await screen.findByRole("dialog", { name: "Preview thumbnail" });
    fireEvent.click(within(dialog).getByRole("radio", { name: "Fill frame" }));
    const stage = within(dialog).getByRole("img", { name: /Drag or use the arrow keys/ });
    fireEvent.keyDown(stage, { key: "+" });
    fireEvent.keyDown(stage, { key: "ArrowDown" });
    expect(within(dialog).getByRole("slider", { name: "Zoom" })).toBeDefined();
    fireEvent.click(within(dialog).getByRole("button", { name: "Use as thumbnail" }));

    await waitFor(() => expect(posts()).toHaveLength(1));
    const [, aspect, fit] = compose.encodeComposition.mock.calls[0] ?? [];
    expect(aspect).toBe(16 / 9);
    expect(fit).toMatchObject({ mode: "cover", zoom: 1.25, panX: 0 });
    expect((fit as { panY: number }).panY).toBeLessThan(0);
  });

  it("送信中は入口が回り、キャプチャと入口を押せず、応答の動画で項目がアップロードした画像になる", async () => {
    let answer: (response: Response) => void = () => undefined;
    server.upload.mockReturnValueOnce(
      new Promise((resolve) => {
        answer = resolve;
      }),
    );
    await openPage();
    const capture = screen.getByRole("button", {
      name: "Use current frame as thumbnail",
    });
    await waitFor(() => expect(capture.getAttribute("aria-disabled")).toBeNull());

    await chooseAndApply();
    await waitFor(() => expect(entry().getAttribute("aria-disabled")).toBe("true"));
    expect(capture.getAttribute("aria-disabled")).toBe("true");
    expect(screen.queryByRole("dialog", { name: "Preview thumbnail" })).toBeNull();
    const form = server.upload.mock.calls[0]?.[0];
    const sent = form?.get("image");
    expect(sent).toBeInstanceOf(File);
    expect((sent as File).name).toBe("thumbnail.png");
    expect((sent as File).type).toBe("image/png");
    expect(form?.get("contentVersion")).toBe("r1");
    expect(compose.encodeComposition.mock.calls[0]?.[2]).toEqual({ mode: "contain" });

    // 送信中に押しても、ピッカーもキャプチャも動かない。
    const input = screen.getByTestId<HTMLInputElement>("thumbnail-upload-input");
    const click = vi.spyOn(input, "click");
    fireEvent.click(entry());
    fireEvent.click(capture);
    expect(click).not.toHaveBeenCalled();
    expect(server.requests.some((r) => r.includes("thumbnail-position"))).toBe(false);

    answer(json(uploaded));
    const item = await within(facts()).findByRole("img", { name: "Uploaded thumbnail" });
    expect(item.getAttribute("src")).toBe("/api/videos/7/thumbnail?v=r1-g3");
    expect(
      within(facts()).queryByRole("img", { name: "Generated thumbnail" }),
    ).toBeNull();
    await waitFor(() => expect(entry().getAttribute("aria-disabled")).toBeNull());
    expect(screen.queryByRole("alert")).toBeNull();
    expect(posts()).toHaveLength(1);
  });

  it("400 では理由の文で失敗の行を出し、サムネイルを変えない", async () => {
    server.upload.mockReturnValueOnce(
      json(
        {
          code: "invalid_request",
          message: "not an image",
          reason: "thumbnail_upload_invalid",
        },
        400,
      ),
    );
    await openPage();
    await chooseAndApply();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't upload the image: The file isn't a PNG, JPEG or still WebP, or it can't be read.",
    );
    expect(within(facts()).queryByRole("img", { name: "Uploaded thumbnail" })).toBeNull();
    await waitFor(() => expect(entry().getAttribute("aria-disabled")).toBeNull());
  });

  it("content_changed では失敗の行を出し、動画を取り直す", async () => {
    server.upload.mockReturnValueOnce(
      json({ code: "conflict", message: "changed", reason: "content_changed" }, 409),
    );
    await openPage();
    const reads = () => server.requests.filter((r) => r === "GET /api/videos/7").length;
    const before = reads();
    await chooseAndApply();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't upload the image: The video changed in the meantime.",
    );
    await waitFor(() => expect(reads()).toBeGreaterThan(before));
  });

  it("アップロードした画像の項目の × は、コマへ戻す要求を送る", async () => {
    server.video = uploaded;
    server.revert.mockReturnValueOnce(json(video));
    await openPage();
    const item = within(facts()).getByRole("img", { name: "Uploaded thumbnail" });
    expect(item.getAttribute("src")).toBe("/api/videos/7/thumbnail?v=r1-g3");

    fireEvent.click(
      within(facts()).getByRole("button", { name: "Use the frame instead" }),
    );
    await waitFor(() =>
      expect(
        within(facts()).queryByRole("img", { name: "Uploaded thumbnail" }),
      ).toBeNull(),
    );
    expect(server.revert).toHaveBeenCalledTimes(1);
  });
});
