import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";

const mock = vi.hoisted(() => {
  type Callback = () => void;
  class FakePlayer {
    handlers = new Map<string, Callback[]>();
    sources: unknown[] = [];
    time = 0;
    pausedValue = true;
    disposed = false;
    errorValue: unknown = null;
    mutedValue = false;
    volumeValue = 1;
    fullscreen = false;
    rate = 1;
    durationValue = 120;
    videoWidthValue = 1920;
    videoHeightValue = 1080;
    posterValue: string | undefined;
    userActiveValue = true;
    element: HTMLElement;
    options: Record<string, unknown>;

    constructor(element: HTMLElement, options: Record<string, unknown>) {
      this.element = element;
      this.options = options;
      this.posterValue = options.poster as string | undefined;
      // 操作バーの DOM だけを真似る（差し込みと aria-keyshortcuts の検査のため）。
      element.innerHTML =
        '<div class="vjs-control-bar"><div class="vjs-progress-control">' +
        '<div class="vjs-progress-holder"></div></div>' +
        '<button class="vjs-play-control"></button>' +
        '<button class="vjs-mute-control"></button><div class="vjs-playback-rate"></div>' +
        '<button class="vjs-fullscreen-control"></button></div>';
    }

    on(event: string, callback: Callback) {
      this.handlers.set(event, [...(this.handlers.get(event) ?? []), callback]);
    }
    ready(callback: Callback) {
      callback();
    }
    videoWidth() {
      return this.videoWidthValue;
    }
    videoHeight() {
      return this.videoHeightValue;
    }
    one(event: string, callback: Callback) {
      const once = () => {
        this.off(event, once);
        callback();
      };
      this.on(event, once);
    }
    off(event: string, callback: Callback) {
      this.handlers.set(
        event,
        (this.handlers.get(event) ?? []).filter((candidate) => candidate !== callback),
      );
    }
    trigger(event: string) {
      for (const callback of [...(this.handlers.get(event) ?? [])]) callback();
    }
    src(source: unknown) {
      this.sources.push(source);
    }
    currentTime(seconds?: number) {
      if (seconds !== undefined) this.time = seconds;
      return this.time;
    }
    paused() {
      return this.pausedValue;
    }
    duration() {
      return this.durationValue;
    }
    muted(value?: boolean) {
      if (value !== undefined) this.mutedValue = value;
      return this.mutedValue;
    }
    volume(value?: number) {
      if (value !== undefined) this.volumeValue = value;
      return this.volumeValue;
    }
    playbackRate(value?: number) {
      if (value !== undefined) this.rate = value;
      return this.rate;
    }
    isFullscreen(value?: boolean) {
      if (value !== undefined) this.fullscreen = value;
      return this.fullscreen;
    }
    requestFullscreen() {
      this.fullscreen = true;
    }
    exitFullscreen() {
      this.fullscreen = false;
    }
    userActive(value?: boolean) {
      if (value !== undefined) this.userActiveValue = value;
      return this.userActiveValue;
    }
    started = false;
    hasStarted(value?: boolean) {
      if (value !== undefined) this.started = value;
      return this.started;
    }
    poster(value?: string) {
      if (value !== undefined) this.posterValue = value;
      return this.posterValue;
    }
    ended() {
      return false;
    }
    play() {
      this.pausedValue = false;
      return Promise.resolve();
    }
    pause() {
      this.pausedValue = true;
    }
    error(value?: unknown) {
      if (arguments.length > 0) this.errorValue = value;
      return this.errorValue;
    }
    isDisposed() {
      return this.disposed;
    }
    dispose() {
      this.disposed = true;
      this.trigger("dispose");
    }
  }

  const instances: FakePlayer[] = [];
  const factory = Object.assign(
    vi.fn((element: HTMLElement, options: Record<string, unknown>) => {
      const player = new FakePlayer(element, options);
      instances.push(player);
      return player;
    }),
    {
      use: vi.fn(),
      addLanguage: vi.fn(),
      createTimeRanges: vi.fn((values: [number, number][]) => ({
        length: values.length,
        start: (index: number) => values[index]?.[0] ?? 0,
        end: (index: number) => values[index]?.[1] ?? 0,
      })),
    },
  );
  return { factory, instances };
});

vi.mock("video.js", () => ({ default: mock.factory }));

import VideoPlayer from "./VideoPlayer";

const video: Video = {
  id: 7,
  title: "test",
  public: false,
  sizeBytes: 100,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "pending",
  durationMs: 120_000,
  videoCodec: "h264",
  container: "mp4",
  tags: [],
};

function props(overrides: Partial<Video> = {}) {
  return {
    video: { ...video, ...overrides },
    initialPositionMs: 0,
    autoplay: false,
    onPosition: vi.fn(),
    onProgress: vi.fn(),
    onError: vi.fn(),
    onControls: vi.fn(),
    onStatus: vi.fn(),
  };
}

describe("VideoPlayer", () => {
  afterEach(() => {
    window.localStorage.clear();
    mock.instances.length = 0;
    vi.clearAllMocks();
  });

  it("保存した音量を復元し、音量の変更を次のプレイヤーへ引き継ぐ", async () => {
    window.localStorage.setItem(
      "vv.playback-volume.v1",
      JSON.stringify({ volume: 0.4, muted: true }),
    );
    const first = render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const firstPlayer = mock.instances[0];
    expect(firstPlayer?.volumeValue).toBe(0.4);
    expect(firstPlayer?.mutedValue).toBe(true);

    if (firstPlayer === undefined) throw new Error("playerがありません");
    firstPlayer.volumeValue = 0.7;
    firstPlayer.mutedValue = false;
    act(() => firstPlayer.trigger("volumechange"));
    expect(
      JSON.parse(window.localStorage.getItem("vv.playback-volume.v1") ?? ""),
    ).toEqual({ volume: 0.7, muted: false });

    first.unmount();
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(2));
    expect(mock.instances[1]?.volumeValue).toBe(0.7);
    expect(mock.instances[1]?.mutedValue).toBe(false);
  });

  it("direct動画はstream、非対応動画はtranscodeから開始する", async () => {
    const direct = render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(mock.instances[0]?.sources[0]).toMatchObject({
      src: "/api/videos/7/stream",
    });
    direct.unmount();

    render(<VideoPlayer {...props({ playable: false })} />);
    await waitFor(() => expect(mock.instances).toHaveLength(2));
    expect(mock.instances[1]?.sources[0]).toMatchObject({
      src: "/api/videos/7/transcode.mp4",
      vvOffsetSeconds: 0,
    });
  });

  it("direct error後だけ同じ論理位置からtranscodeへ切り替える", async () => {
    const values = props();
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    player.time = 12.345;
    player.pausedValue = false;
    player.trigger("play");
    player.trigger("error");

    expect(player.sources).toHaveLength(2);
    expect(player.sources[1]).toMatchObject({
      src: "/api/videos/7/transcode.mp4?startMs=12345",
      vvOffsetSeconds: 12.345,
    });
    player.trigger("error");
    expect(player.sources).toHaveLength(2);
    expect(values.onError).toHaveBeenCalledTimes(1);
  });

  it("metadata前のdirect errorでも保存位置からtranscodeへ切り替える", async () => {
    const values = { ...props(), initialPositionMs: 12_345 };
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");

    player.trigger("error");

    expect(player.sources[1]).toMatchObject({
      src: "/api/videos/7/transcode.mp4?startMs=12345",
      vvOffsetSeconds: 12.345,
    });
  });

  it("pause時に論理位置を保存し、unmount時にdisposeする", async () => {
    const values = props();
    const view = render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    player.trigger("loadedmetadata");
    player.time = 24.2;
    player.trigger("pause");
    expect(values.onProgress).toHaveBeenCalledWith(24_200, true);

    view.unmount();
    expect(player.disposed).toBe(true);
  });

  it("操作バーは速度・現在時刻/長さを持ち、秒数送りと残り時間を持たない", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const options = mock.instances[0]?.options as {
      playbackRates: number[];
      controlBar: {
        children: string[];
        skipButtons?: unknown;
        remainingTimeDisplay: boolean;
      };
    };
    expect(options.playbackRates).toEqual([0.5, 0.75, 1, 1.25, 1.5, 2]);
    expect(options.controlBar.skipButtons).toBeUndefined();
    expect(options.controlBar.remainingTimeDisplay).toBe(false);
    expect(options.controlBar.children).not.toContain("remainingTimeDisplay");
    expect(options.controlBar.children.slice(0, 5)).toEqual([
      "playToggle",
      "volumePanel",
      "currentTimeDisplay",
      "timeDivider",
      "durationDisplay",
    ]);
    expect(options.controlBar.children.slice(-3)).toEqual([
      "playbackRateMenuButton",
      "pictureInPictureToggle",
      "fullscreenToggle",
    ]);
    const element = mock.instances[0]?.element;
    expect(
      element?.querySelector(".vjs-play-control")?.getAttribute("aria-keyshortcuts"),
    ).toBe("Space");
  });

  it("操作バーの再生の前に「最初に戻る」を差し込み、押すと先頭へ戻る", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    const restart = await screen.findByRole("button", { name: "最初に戻る" });
    const labels = Array.from(
      player?.element.querySelectorAll(".vjs-control-bar button") ?? [],
      (button) => button.getAttribute("aria-label") ?? button.className,
    );
    expect(labels.slice(0, 2)).toEqual(["最初に戻る", "vjs-play-control"]);
    expect(restart.getAttribute("aria-keyshortcuts")).toBe("0");

    player?.currentTime(42);
    fireEvent.click(restart);
    expect(player?.time).toBe(0);
  });

  it("変換して再生する動画だけ、操作バーの再生速度の前に「変換して再生中」を出す", async () => {
    const direct = render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(screen.queryByText("変換して再生中")).toBeNull();
    direct.unmount();

    render(<VideoPlayer {...props({ playable: false })} />);
    const indicator = await screen.findByRole("button", { name: "変換して再生中" });
    const bar = mock.instances[1]?.element.querySelector(".vjs-control-bar");
    expect(bar?.contains(indicator)).toBe(true);
    expect(
      indicator.closest(".vv-transcode-indicator")?.nextElementSibling?.className,
    ).toBe("vjs-playback-rate");
    fireEvent.click(indicator);
    expect(await screen.findByText(/シークに数秒かかります/)).toBeDefined();
  });

  it("directからtranscodeへ切り替えたら「変換して再生中」を出す", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    act(() => mock.instances[0]?.trigger("error"));
    expect(await screen.findByRole("button", { name: "変換して再生中" })).toBeDefined();
  });

  it("thumbnailStateだけが変わってもプレイヤーを作り直さず、背景の画像だけ差し替える", async () => {
    const values = props({ thumbnailState: "pending", seekThumbnailState: "pending" });
    const view = render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));

    view.rerender(
      <VideoPlayer
        {...values}
        video={{
          ...values.video,
          thumbnailState: "done",
          thumbnailUrl: "/api/videos/7/thumbnail?v=1",
          seekThumbnailState: "done",
          previewState: "done",
          progress: {
            positionMs: 30_000,
            completed: false,
            updatedAt: "2026-09-02T00:00:00Z",
          },
        }}
        initialPositionMs={30_000}
      />,
    );
    expect(mock.instances).toHaveLength(1);
    expect(mock.instances[0]?.disposed).toBe(false);
    expect(mock.instances[0]?.posterValue).toBe("/api/videos/7/thumbnail?v=1");
  });

  it("読み取りが終わって再生できるようになったら作り直す", async () => {
    const values = props();
    const view = render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    view.rerender(
      <VideoPlayer {...values} video={{ ...values.video, durationMs: 130_000 }} />,
    );
    expect(mock.instances).toHaveLength(2);
    expect(mock.instances[0]?.disposed).toBe(true);
  });

  it("最後の失敗は論理位置を渡し、操作バーを隠す誤りの印を消す", async () => {
    const values = props({ playable: false });
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    player.trigger("loadedmetadata");
    player.time = 42.5;
    player.errorValue = { code: 4 };
    act(() => player.trigger("error"));
    expect(values.onError).toHaveBeenCalledWith(42_500);
    expect(player.errorValue).toBeNull();
    // 変換へ切り替えたあとの失敗でも、操作バーを出す印を付け直す。
    expect(player.started).toBe(true);
  });

  it("読み込んだ映像の比率を知らせる", async () => {
    const onAspectRatio = vi.fn();
    render(<VideoPlayer {...props()} onAspectRatio={onAspectRatio} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    player.videoWidthValue = 1080;
    player.videoHeightValue = 1920;
    act(() => player.trigger("loadedmetadata"));
    expect(onAspectRatio).toHaveBeenCalledWith(1080 / 1920);
  });

  it("autoplayなら作ってすぐ再生を始める", async () => {
    render(<VideoPlayer {...props()} autoplay />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(mock.instances[0]?.pausedValue).toBe(false);
  });

  it("操作の入口を渡し、先頭から・ミュート・全画面を動かす", async () => {
    const values = props();
    const view = render(<VideoPlayer {...values} />);
    await waitFor(() => expect(values.onControls).toHaveBeenCalled());
    const controls = values.onControls.mock.calls.at(-1)?.[0] as
      import("./playerControls").PlayerControls | null;
    const player = mock.instances[0];
    if (controls == null || player === undefined)
      throw new Error("操作の入口がありません");

    player.time = 30;
    controls.restart();
    expect(player.time).toBe(0);
    expect(player.pausedValue).toBe(false);
    controls.togglePlay();
    expect(player.pausedValue).toBe(true);
    controls.toggleMute();
    expect(player.mutedValue).toBe(true);
    controls.toggleFullscreen();
    expect(controls.isFullscreen()).toBe(true);

    view.unmount();
    expect(values.onControls).toHaveBeenLastCalledWith(null);
  });

  it("ended・play・user-activeの変化を状態として知らせる", async () => {
    const values = props();
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    act(() => player.trigger("play"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ playing: true, ended: false }),
    );
    act(() => player.trigger("userinactive"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ userActive: false }),
    );
    act(() => player.trigger("ended"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ playing: false, ended: true }),
    );
    act(() => player.trigger("seeking"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ ended: false }),
    );
  });

  it("全画面は上に重ねる層ごと（渡した入れ物）にし、吹き出しもその中に描く", async () => {
    const frame = document.createElement("div");
    document.body.append(frame);
    const requestFullscreen = vi.fn(() => Promise.resolve());
    Object.assign(frame, { requestFullscreen });
    const values = { ...props({ playable: false }), fullscreenTarget: () => frame };
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(values.onControls).toHaveBeenCalled());
    const controls = values.onControls.mock.calls.at(-1)?.[0] as
      import("./playerControls").PlayerControls | null;
    const player = mock.instances[0];
    if (controls == null || player === undefined)
      throw new Error("操作の入口がありません");
    const changes = vi.fn();
    player.on("fullscreenchange", changes);

    // F キーも video.js の全画面ボタンも player.requestFullscreen を通る。
    controls.toggleFullscreen();
    expect(requestFullscreen).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      value: frame,
    });
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(controls.isFullscreen()).toBe(true);
    // 全画面ボタンの表示を切り替えるため、プレイヤーにも知らせる。
    expect(changes).toHaveBeenCalledTimes(1);

    fireEvent.click(await screen.findByRole("button", { name: "変換して再生中" }));
    const content = await screen.findByText(/シークに数秒かかります/);
    expect(frame.contains(content)).toBe(true);

    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      value: null,
    });
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(controls.isFullscreen()).toBe(false);
    frame.remove();
  });

  it("シーク位置サムネイルの URL が後から来たら、作り直さずに再生バーへ付ける", async () => {
    const values = props({ seekThumbnailUrl: undefined, seekThumbnailState: "pending" });
    const view = render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const element = mock.instances[0]?.element;
    expect(element?.querySelector(".vv-seek-preview")).toBeNull();

    view.rerender(
      <VideoPlayer
        {...values}
        video={{
          ...values.video,
          seekThumbnailUrl: "/api/videos/7/seek-thumbnail?v=1",
          seekThumbnailState: "done",
        }}
      />,
    );
    await waitFor(() =>
      expect(
        element?.querySelector(".vjs-progress-holder .vv-seek-preview"),
      ).not.toBeNull(),
    );
    expect(mock.instances).toHaveLength(1);
  });

  it("読み込みの前に最後の失敗が起きても、操作バーを隠したままにしない", async () => {
    const values = props({ playable: false });
    const view = render(<VideoPlayer {...values} autoplay />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const host = view.container.querySelector(".vv-video-player");
    expect(host?.getAttribute("data-loading")).toBe("true");
    act(() => mock.instances[0]?.trigger("error"));
    expect(values.onError).toHaveBeenCalled();
    expect(host?.hasAttribute("data-loading")).toBe(false);
  });
});
