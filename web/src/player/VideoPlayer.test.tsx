import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";

const mock = vi.hoisted(() => {
  type Callback = (event?: unknown, hash?: unknown) => void;
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
    /** 付けた字幕のトラック（addRemoteTextTrack の引数と mode）。 */
    remoteTracks: {
      options: { src: string; label: string; kind: string };
      mode: string;
    }[] = [];
    trackListeners: Callback[] = [];
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
        '<button class="vjs-mute-control"></button>' +
        '<div class="vjs-subs-caps-button"><button class="vjs-subs-caps-button"></button></div>' +
        '<div class="vjs-playback-rate"></div>' +
        '<button class="vjs-fullscreen-control"></button></div>';
    }

    textTracks() {
      return {
        addEventListener: (_type: string, listener: Callback) => {
          this.trackListeners.push(listener);
        },
        removeEventListener: (_type: string, listener: Callback) => {
          this.trackListeners = this.trackListeners.filter((item) => item !== listener);
        },
      };
    }
    addRemoteTextTrack(options: { src: string; label: string; kind: string }) {
      const track = { options, mode: "disabled" };
      this.remoteTracks.push(track);
      return { track };
    }
    removeRemoteTextTrack(track: unknown) {
      this.remoteTracks = this.remoteTracks.filter((item) => item !== track);
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
    trigger(event: string, hash?: unknown) {
      for (const callback of [...(this.handlers.get(event) ?? [])])
        callback({ type: event }, hash);
    }
    /** 画質メニューに渡した状態（qualityMenu.ts の setQualityMenu）。 */
    qualityStates: { options: readonly string[]; current: string }[] = [];
    getChild(name: string): unknown {
      if (name === "ControlBar") return this;
      if (name === "QualityMenuButton") {
        return {
          setQualityState: (state: { options: readonly string[]; current: string }) => {
            this.qualityStates.push(state);
          },
        };
      }
      return undefined;
    }
    src(source: unknown) {
      this.sources.push(source);
    }
    /** 技術層の要素。設けたときだけ、要素が読み込んでいる source（currentSrc）を確かめる。 */
    techElement: HTMLVideoElement | undefined;
    tech() {
      const element = this.techElement;
      return element === undefined ? undefined : { el: () => element };
    }
    /** 要素が読み込んでいる source を src に替える。 */
    loadInTech(src: string) {
      this.techElement ??= document.createElement("video");
      Object.defineProperty(this.techElement, "currentSrc", {
        value: new URL(src, window.location.href).href,
        configurable: true,
      });
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
      // qualityMenu.ts が読み込むときに基底の部品を引いて登録する。
      getComponent: vi.fn(() => class {}),
      registerComponent: vi.fn(),
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

import VideoPlayer, { playerLanguage } from "./VideoPlayer";

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
    vi.unstubAllGlobals();
    vi.useRealTimers();
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
    player.errorValue = { code: 3 };
    player.trigger("error");

    expect(player.sources).toHaveLength(2);
    expect(player.sources[1]).toMatchObject({
      src: expect.stringMatching(
        /^\/api\/videos\/7\/transcode\.mp4\?startMs=12345&attempt=[0-9a-f]{32}$/,
      ) as unknown,
      vvOffsetSeconds: 12.345,
    });
    player.errorValue = { code: 3 };
    player.trigger("error");
    expect(player.sources).toHaveLength(2);
    expect(values.onError).toHaveBeenCalledTimes(1);
    expect(values.onError).toHaveBeenCalledWith(12_345, "decode");
  });

  it("metadata前のdirect errorでも保存位置からtranscodeへ切り替える", async () => {
    const values = { ...props(), initialPositionMs: 12_345 };
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");

    player.errorValue = { code: 3 };
    player.trigger("error");

    expect(player.sources[1]).toMatchObject({
      src: expect.stringMatching(
        /^\/api\/videos\/7\/transcode\.mp4\?startMs=12345&attempt=[0-9a-f]{32}$/,
      ) as unknown,
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

  it("操作バーは画質・速度・現在時刻/長さを持ち、秒数送りと残り時間を持たない", async () => {
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
    expect(options.controlBar.children.slice(-5)).toEqual([
      "qualityMenuButton",
      "subtitlesButton",
      "playbackRateMenuButton",
      "pictureInPictureToggle",
      "fullscreenToggle",
    ]);
    const element = mock.instances[0]?.element;
    expect(
      element?.querySelector(".vjs-play-control")?.getAttribute("aria-keyshortcuts"),
    ).toBe("Space");
  });

  it("字幕ボタンを再生速度の前に置き、字幕の設定を出さず、C キーを添える", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const options = mock.instances[0]?.options as {
      textTrackSettings: boolean;
      controlBar: { children: string[] };
    };
    expect(options.textTrackSettings).toBe(false);
    const children = options.controlBar.children;
    expect(children.indexOf("subtitlesButton")).toBe(
      children.indexOf("playbackRateMenuButton") - 1,
    );
    const element = mock.instances[0]?.element;
    expect(
      element
        ?.querySelector("button.vjs-subs-caps-button")
        ?.getAttribute("aria-keyshortcuts"),
    ).toBe("C");
  });

  it("字幕の一覧をトラックとして付け、一覧が届き直してもプレイヤーを作り直さない", async () => {
    window.localStorage.setItem(
      "vv.subtitles.v1",
      JSON.stringify({ enabled: true, label: "ja" }),
    );
    const base = props();
    const view = render(<VideoPlayer {...base} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    expect(player?.remoteTracks).toEqual([]);

    view.rerender(
      <VideoPlayer
        {...base}
        subtitles={[
          { file: "movie.srt", label: "", format: "srt" },
          { file: "movie.ja.srt", label: "ja", format: "srt" },
        ]}
      />,
    );
    await waitFor(() => expect(player?.remoteTracks).toHaveLength(2));
    expect(mock.instances).toHaveLength(1);
    expect(player?.remoteTracks.map((track) => track.options)).toEqual([
      {
        kind: "subtitles",
        src: "/api/videos/7/subtitles/movie.srt",
        label: "Default",
        default: false,
      },
      {
        kind: "subtitles",
        src: "/api/videos/7/subtitles/movie.ja.srt",
        label: "ja",
        default: false,
      },
    ]);
    expect(player?.remoteTracks.map((track) => track.mode)).toEqual([
      "disabled",
      "showing",
    ]);

    // c キーの入口は字幕を切り替える。
    const controls = base.onControls.mock.calls.at(-1)?.[0] as {
      toggleSubtitles(): void;
    };
    act(() => controls.toggleSubtitles());
    expect(player?.remoteTracks.map((track) => track.mode)).toEqual([
      "disabled",
      "disabled",
    ]);
    expect(JSON.parse(window.localStorage.getItem("vv.subtitles.v1") ?? "null")).toEqual({
      enabled: false,
      label: "ja",
    });

    view.rerender(<VideoPlayer {...base} subtitles={[]} />);
    await waitFor(() => expect(player?.remoteTracks).toEqual([]));
    expect(mock.instances).toHaveLength(1);
  });

  it("直接再生から変換へ切り替えると、offset が決まるまで字幕を外し、決まった offset で付け直す", async () => {
    window.localStorage.setItem(
      "vv.subtitles.v1",
      JSON.stringify({ enabled: true, label: "ja" }),
    );
    render(
      <VideoPlayer
        {...props()}
        subtitles={[{ file: "movie.ja.srt", label: "ja", format: "srt" }]}
      />,
    );
    await waitFor(() => expect(mock.instances[0]?.remoteTracks).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    expect(player.remoteTracks[0]?.options.src).toBe(
      "/api/videos/7/subtitles/movie.ja.srt",
    );

    player.time = 12.345;
    player.errorValue = { code: 3 };
    act(() => player.trigger("error"));
    const live = player.sources[1] as {
      vvOffsetSettled?: (seconds: number) => void;
      vvOffsetPending?: () => void;
    };
    // 仲立ちは報告を待ち始めたときに未決を、届いたら実際の開始位置を知らせる。
    act(() => live.vvOffsetPending?.());
    expect(player.remoteTracks).toEqual([]);
    act(() => live.vvOffsetSettled?.(8));
    expect(player.remoteTracks.map((track) => track.options.src)).toEqual([
      "/api/videos/7/subtitles/movie.ja.srt?offsetMs=8000",
    ]);
    expect(player.remoteTracks[0]?.mode).toBe("showing");
    expect(JSON.parse(window.localStorage.getItem("vv.subtitles.v1") ?? "null")).toEqual({
      enabled: true,
      label: "ja",
    });
  });

  it("操作バーの読み上げ名とツールチップは英語の独自言語で、キーボード操作を持つボタンにキーを添える", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(playerLanguage).toBe("en-x-vv");
    expect(mock.instances[0]?.options.language).toBe(playerLanguage);
    expect(mock.factory.addLanguage).toHaveBeenCalledWith(playerLanguage, {
      Play: "Play (Space)",
      Pause: "Pause (Space)",
      Replay: "Replay (Space)",
      "Play Video": "Play",
      Mute: "Mute (M)",
      Unmute: "Unmute (M)",
      Fullscreen: "Fullscreen (F)",
      "Exit Fullscreen": "Exit fullscreen (F)",
      "Picture-in-Picture": "Picture-in-picture",
      "Exit Picture-in-Picture": "Exit picture-in-picture",
      "Playback Rate": "Playback speed",
      "Current Time": "Current time",
      Duration: "Duration",
      "Progress Bar": "Playback position",
      "Volume Level": "Volume",
      "Video Player": "Video player",
      ", selected": ", selected",
      Subtitles: "Subtitles (C)",
      Captions: "Subtitles (C)",
      "subtitles off": "Off",
      "captions off": "Off",
      "captions and subtitles off": "Off",
    });
    const element = mock.instances[0]?.element;
    const shortcuts = [
      ".vjs-play-control",
      ".vjs-mute-control",
      ".vjs-fullscreen-control",
    ].map((selector) =>
      element?.querySelector(selector)?.getAttribute("aria-keyshortcuts"),
    );
    expect(shortcuts).toEqual(["Space", "M", "F"]);
    const restart = await screen.findByRole("button", { name: "Restart" });
    expect(restart.getAttribute("title")).toBe("Restart (0)");
  });

  it("疑似ロケールで、video.js の独自言語・差し込んだ操作・変換の案内がカタログから出る", async () => {
    enablePseudoLocale();
    render(<VideoPlayer {...props({ playable: false })} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const dictionary = mock.factory.addLanguage.mock.calls.at(-1)?.[1] as Record<
      string,
      string
    >;
    for (const text of Object.values(dictionary)) {
      expect(text.startsWith("⟦") && text.endsWith("⟧")).toBe(true);
    }
    fireEvent.click(
      await screen.findByRole("button", { name: /Converting for playback/ }),
    );
    await screen.findByText(/Seeking takes a few seconds/);
    expectCatalogTextOnly(document.body);
  });

  it("操作バーの再生の前に「最初に戻る」を差し込み、押すと先頭へ戻る", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    const restart = await screen.findByRole("button", { name: "Restart" });
    const labels = Array.from(
      player?.element.querySelectorAll(".vjs-control-bar button") ?? [],
      (button) => button.getAttribute("aria-label") ?? button.className,
    );
    expect(labels.slice(0, 2)).toEqual(["Restart", "vjs-play-control"]);
    expect(restart.getAttribute("aria-keyshortcuts")).toBe("0");

    player?.currentTime(42);
    fireEvent.click(restart);
    expect(player?.time).toBe(0);
  });

  it("変換して再生する動画だけ、操作バーの再生速度の前に「変換して再生中」を出す", async () => {
    const direct = render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(screen.queryByText("Converting for playback")).toBeNull();
    direct.unmount();

    render(<VideoPlayer {...props({ playable: false })} />);
    const indicator = await screen.findByRole("button", {
      name: "Converting for playback",
    });
    const bar = mock.instances[1]?.element.querySelector(".vjs-control-bar");
    expect(bar?.contains(indicator)).toBe(true);
    // 字幕ボタンは再生速度の隣に保ち、「変換して再生中」はその前に置く。
    const next = indicator.closest(".vv-transcode-indicator")?.nextElementSibling;
    expect(next?.className).toBe("vjs-subs-caps-button");
    expect(next?.nextElementSibling?.className).toBe("vjs-playback-rate");
    fireEvent.click(indicator);
    expect(await screen.findByText(/Seeking takes a few seconds/)).toBeDefined();
  });

  it("覚えた画質で直接再生できる動画も変換で始め、その画質で変換していると出す", async () => {
    window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
    render(<VideoPlayer {...props({ width: 1920, height: 1080 })} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(mock.instances[0]?.sources[0]).toMatchObject({
      src: "/api/videos/7/transcode.mp4?quality=480p",
      vvQuality: "480p",
      vvOffsetSeconds: 0,
    });
    const indicator = await screen.findByRole("button", { name: "Converting to 480p" });
    expect(screen.queryByText("Converting for playback")).toBeNull();
    fireEvent.click(indicator);
    expect(
      await screen.findByText(/Playing a 480p version converted while it plays/),
    ).toBeDefined();
  });

  it("覚えた画質がこの動画に使えなければ元の画質で再生し、覚えた値は変えない", async () => {
    window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
    render(<VideoPlayer {...props({ width: 640, height: 360 })} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    expect(mock.instances[0]?.sources[0]).toMatchObject({ src: "/api/videos/7/stream" });
    expect(screen.queryByText(/Converting/)).toBeNull();
    expect(window.localStorage.getItem("vv.playback-quality.v1")).toBe(
      JSON.stringify("480p"),
    );
  });

  it("directからtranscodeへ切り替えたら「変換して再生中」を出す", async () => {
    render(<VideoPlayer {...props()} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    act(() => {
      const player = mock.instances[0];
      if (player === undefined) return;
      player.errorValue = { code: 3 };
      player.trigger("error");
    });
    expect(
      await screen.findByRole("button", { name: "Converting for playback" }),
    ).toBeDefined();
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
    // 番号 4 はサーバーに届くかを確かめてから分ける。届けば動画を出せなかった失敗になる。
    const fetchMock = vi.fn(() => Promise.resolve(new Response("{}")));
    vi.stubGlobal("fetch", fetchMock);
    act(() => player.trigger("error"));
    await waitFor(() => expect(values.onError).toHaveBeenCalledWith(42_500, "source"));
    expect(fetchMock).toHaveBeenCalledWith("/api/health", expect.anything());
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

  it("最初のメタデータで続きからの位置を当て終えたら positioned を知らせ、操作の入口は今の論理上の位置を返す", async () => {
    const values = { ...props(), initialPositionMs: 12_000 };
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(values.onControls).toHaveBeenCalled());
    const controls = values.onControls.mock.calls.at(-1)?.[0] as
      import("./playerControls").PlayerControls | null;
    const player = mock.instances[0];
    if (controls == null || player === undefined)
      throw new Error("操作の入口がありません");
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ positioned: false }),
    );

    act(() => player.trigger("loadedmetadata"));
    expect(player.time).toBe(12);
    // 続きからの位置へのシークが終わるまでは、映っている場面がまだその位置ではない。
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ positioned: false }),
    );
    act(() => player.trigger("seeked"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ positioned: true }),
    );
    player.time = 83.4564;
    expect(controls.positionMs()).toBe(83_456);
    expect(values.onPosition).toHaveBeenLastCalledWith(controls.positionMs());
  });

  it("変換で続きから始めたときは、実際の開始位置の報告が届くまで positioned を立てない", async () => {
    const values = { ...props({ playable: false }), initialPositionMs: 12_000 };
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    const live = player.sources[0] as {
      vvOffsetSettled?: (seconds: number) => void;
      vvOffsetPending?: () => void;
    };
    // 仲立ちは source を設定した直後に報告を待ち始める。
    act(() => live.vvOffsetPending?.());
    act(() => player.trigger("loadedmetadata"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ positioned: false }),
    );

    act(() => live.vvOffsetSettled?.(8));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ positioned: true }),
    );
  });

  it("続きの位置が無ければ最初のメタデータで positioned を立てる", async () => {
    const values = props();
    render(<VideoPlayer {...values} />);
    await waitFor(() => expect(mock.instances).toHaveLength(1));
    const player = mock.instances[0];
    if (player === undefined) throw new Error("playerがありません");
    act(() => player.trigger("loadedmetadata"));
    expect(values.onStatus).toHaveBeenLastCalledWith(
      expect.objectContaining({ positioned: true }),
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

    fireEvent.click(
      await screen.findByRole("button", { name: "Converting for playback" }),
    );
    const content = await screen.findByText(/Seeking takes a few seconds/);
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
    act(() => {
      const player = mock.instances[0];
      if (player === undefined) return;
      player.errorValue = { code: 3 };
      player.trigger("error");
    });
    expect(values.onError).toHaveBeenCalled();
    expect(host?.hasAttribute("data-loading")).toBe(false);
  });

  describe("通信が切れたとき", () => {
    function lastStatus(values: ReturnType<typeof props>) {
      const calls = values.onStatus.mock.calls;
      return calls[calls.length - 1]?.[0] as Record<string, unknown> | undefined;
    }

    function startPlaying(values: ReturnType<typeof props>) {
      render(<VideoPlayer {...values} />);
      const player = mock.instances[0];
      if (player === undefined) throw new Error("playerがありません");
      player.trigger("loadedmetadata");
      player.time = 30;
      player.pausedValue = false;
      player.trigger("play");
      return player;
    }

    it("変換へ切り替えず、待ってから同じ位置で直接再生を読み込み直し、再生を続ける", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));

      expect(player.errorValue).toBeNull();
      expect(lastStatus(values)).toMatchObject({ reconnecting: true, loading: false });
      expect(player.sources).toHaveLength(1);
      expect(values.onError).not.toHaveBeenCalled();

      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources).toHaveLength(2);
      expect(player.sources[1]).toEqual({
        src: "/api/videos/7/stream",
        type: "video/mp4",
      });

      player.time = 0;
      player.pausedValue = true;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(30);
      expect(player.pausedValue).toBe(false);
      expect(lastStatus(values)).toMatchObject({ reconnecting: false });
      expect(
        screen.queryByRole("button", { name: "Converting for playback" }),
      ).toBeNull();
    });

    it("変換中に切れたら、切れた位置から変換を始め直す", () => {
      vi.useFakeTimers();
      const values = props({ playable: false });
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources[1]).toMatchObject({
        src: expect.stringMatching(
          /^\/api\/videos\/7\/transcode\.mp4\?startMs=30000&attempt=[0-9a-f]{32}$/,
        ) as unknown,
        vvOffsetSeconds: 30,
      });
    });

    it("覚えた画質で変換中に切れたら、同じ画質で切れた位置から変換を始め直す", () => {
      vi.useFakeTimers();
      window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
      const values = props({ width: 1920, height: 1080 });
      const player = startPlaying(values);
      expect(player.sources[0]).toMatchObject({
        src: "/api/videos/7/transcode.mp4?quality=480p",
      });
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources[1]).toMatchObject({
        src: expect.stringMatching(
          /^\/api\/videos\/7\/transcode\.mp4\?startMs=30000&attempt=[0-9a-f]{32}&quality=480p$/,
        ) as unknown,
        vvOffsetSeconds: 30,
        vvQuality: "480p",
      });
    });

    it("読み込み直しを使い切ったら、通信の失敗として伝える", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      for (let round = 0; round < 5; round += 1) {
        player.errorValue = { code: 2 };
        act(() => player.trigger("error"));
        act(() => vi.advanceTimersByTime(15_000));
      }
      expect(values.onError).not.toHaveBeenCalled();
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      expect(values.onError).toHaveBeenCalledWith(30_000, "network");
      expect(lastStatus(values)).toMatchObject({ reconnecting: false, playing: false });
    });

    it("読み込み直したあと十分に進んだら、回数を数え直す", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      for (let round = 0; round < 5; round += 1) {
        player.errorValue = { code: 2 };
        act(() => player.trigger("error"));
        act(() => vi.advanceTimersByTime(15_000));
        act(() => player.trigger("loadedmetadata"));
      }
      for (let second = 31; second <= 41; second += 1) {
        player.time = second;
        act(() => player.trigger("timeupdate"));
      }
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      expect(values.onError).not.toHaveBeenCalled();
      expect(lastStatus(values)).toMatchObject({ reconnecting: true });
    });

    it("シークで進んだ分は再生した長さに数えず、回数を数え直さない", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      for (let round = 0; round < 5; round += 1) {
        player.errorValue = { code: 2 };
        act(() => player.trigger("error"));
        act(() => vi.advanceTimersByTime(15_000));
        act(() => player.trigger("loadedmetadata"));
      }
      act(() => player.trigger("seeking"));
      player.time = 60;
      act(() => player.trigger("timeupdate"));
      player.time = 61;
      act(() => player.trigger("timeupdate"));
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      expect(values.onError).toHaveBeenCalledWith(61_000, "network");
    });

    it("一時停止している間の位置の変化は、再生した長さに数えない", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      for (let round = 0; round < 5; round += 1) {
        player.errorValue = { code: 2 };
        act(() => player.trigger("error"));
        act(() => vi.advanceTimersByTime(15_000));
        act(() => player.trigger("loadedmetadata"));
      }
      act(() => player.pause());
      act(() => player.trigger("pause"));
      for (let second = 31; second <= 45; second += 1) {
        player.time = second;
        act(() => player.trigger("timeupdate"));
      }
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      expect(values.onError).toHaveBeenCalledWith(45_000, "network");
    });

    it("読み込み直した要求が誤りも出さずに止まったら、期限を過ぎて次の読み込み直しへ進む", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources).toHaveLength(2);
      act(() => vi.advanceTimersByTime(15_000));
      expect(lastStatus(values)).toMatchObject({ reconnecting: true });
      act(() => vi.advanceTimersByTime(2000));
      expect(player.sources).toHaveLength(3);
    });

    it("止まった要求を使い切ったら、通信の失敗として伝える", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(5 * 15_000 + 30_000));
      expect(values.onError).toHaveBeenCalledWith(30_000, "network");
    });

    it("待つ間に一時停止したら、読み込み直したあとも止めたままにする", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => player.pause());
      act(() => vi.advanceTimersByTime(1000));
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(true);
      expect(lastStatus(values)).toMatchObject({ reconnecting: false });
    });

    it("待つ間に再生を押したら、待たずに読み込み直して再生を続ける", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => player.pause());
      act(() => {
        void player.play();
      });
      expect(player.sources).toHaveLength(2);
      player.pausedValue = true;
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(false);
    });

    it("止まった要素が再生中と答えても、待つ間は意図を返し、再生の切り替えは意図に従う", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      // 要素は止まっていない（pausedValue は偽）が、見る人の意図は再生である。
      expect(player.paused()).toBe(false);
      const controls = values.onControls.mock.calls[0]?.[0] as {
        togglePlay(): void;
      };
      act(() => controls.togglePlay());
      expect(player.paused()).toBe(true);
      act(() => controls.togglePlay());
      expect(player.paused()).toBe(false);
      // 再生を押したので、待たずに読み込み直した。
      expect(player.sources).toHaveLength(2);
    });

    it("待つ間のシークは、その位置から直接再生を読み込み直す", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      expect(player.currentTime(60)).toBe(60);
      expect(values.onPosition).toHaveBeenLastCalledWith(60_000);
      act(() => vi.advanceTimersByTime(1000));
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(60);
    });

    it("待つ間のシークは、その位置から変換を始め直す", () => {
      vi.useFakeTimers();
      const values = props({ playable: false });
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => player.currentTime(60));
      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources[1]).toMatchObject({ vvOffsetSeconds: 60 });
    });

    it("読み込み直しの要求の途中でシークしたら、メタデータの後にその位置へ移る", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources).toHaveLength(2);
      act(() => player.currentTime(75));
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(75);
    });

    it("変換の読み込み直しの途中でシークしたら、メタデータの後にその位置へシークし直す", () => {
      vi.useFakeTimers();
      const values = props({ playable: false });
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      expect(player.sources[1]).toMatchObject({ vvOffsetSeconds: 30 });
      act(() => player.currentTime(75));
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(75);
    });

    it("変換の読み込み直しの途中でシークしなければ、実際の開始位置の知らせでもシークしない", () => {
      vi.useFakeTimers();
      const values = props({ playable: false });
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      // 仲立ちが、コピーで始めた変換の実際の開始位置（直前のキーフレーム）を知らせる。
      const reloaded = player.sources[1] as { vvOffsetChanged(seconds: number): void };
      act(() => reloaded.vvOffsetChanged(28.5));
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(0);
    });

    it("直接再生でファイルを出せないと分かったら、変換へ切り替えずに伝える", async () => {
      const values = props();
      const player = startPlaying(values);
      const fetchMock = vi.fn(() => Promise.resolve(new Response("", { status: 404 })));
      vi.stubGlobal("fetch", fetchMock);
      player.errorValue = { code: 4 };
      act(() => player.trigger("error"));
      await waitFor(() => expect(values.onError).toHaveBeenCalledWith(30_000, "source"));
      expect(player.sources).toHaveLength(1);
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/videos/7/stream",
        expect.objectContaining({ headers: { Range: "bytes=0-0" } }),
      );
    });

    it("端末が回線に戻ったら、待たずに読み込み直す", () => {
      vi.useFakeTimers();
      const values = props();
      const player = startPlaying(values);
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      act(() => vi.advanceTimersByTime(1000));
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      expect(player.sources).toHaveLength(2);
      act(() => window.dispatchEvent(new Event("online")));
      expect(player.sources).toHaveLength(3);
    });

    it("番号だけで決まらない誤りは、サーバーに届かなければ通信の失敗として読み込み直す", async () => {
      const values = props();
      const player = startPlaying(values);
      vi.stubGlobal(
        "fetch",
        vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))),
      );
      player.errorValue = { code: 4 };
      act(() => player.trigger("error"));
      expect(lastStatus(values)).toMatchObject({ loading: true });
      await waitFor(() =>
        expect(lastStatus(values)).toMatchObject({ reconnecting: true }),
      );
      expect(player.sources).toHaveLength(1);
      expect(values.onError).not.toHaveBeenCalled();
    });

    it("番号だけで決まらない誤りは、サーバーに届けば直接再生から変換へ切り替える", async () => {
      const values = props();
      const player = startPlaying(values);
      vi.stubGlobal(
        "fetch",
        vi.fn(() => Promise.resolve(new Response("{}"))),
      );
      player.errorValue = { code: 4 };
      act(() => player.trigger("error"));
      await waitFor(() => expect(player.sources).toHaveLength(2));
      expect(player.sources[1]).toMatchObject({ vvOffsetSeconds: 30 });
    });

    it("捨てたプレイヤーは読み込み直さない", () => {
      vi.useFakeTimers();
      const values = props();
      render(<VideoPlayer {...values} />);
      const player = mock.instances[0];
      if (player === undefined) throw new Error("playerがありません");
      player.errorValue = { code: 2 };
      act(() => player.trigger("error"));
      cleanup();
      act(() => vi.advanceTimersByTime(20_000));
      expect(player.sources).toHaveLength(1);
    });
  });

  describe("画質の切り替え", () => {
    const hd = { width: 1920, height: 1080 };

    async function playerFor(values: ReturnType<typeof props>) {
      render(<VideoPlayer {...values} />);
      await waitFor(() => expect(mock.instances).toHaveLength(1));
      const player = mock.instances[0];
      if (player === undefined) throw new Error("playerがありません");
      act(() => player.trigger("loadedmetadata"));
      return player;
    }

    function select(player: (typeof mock.instances)[number], quality: string) {
      act(() => player.trigger("vvqualityselect", { quality }));
    }

    it("再生中に480pを選ぶと、作り直さずに同じ位置から480pの変換で再生を続け、画質を覚える", async () => {
      const values = props(hd);
      const player = await playerFor(values);
      expect(player.qualityStates.at(-1)).toMatchObject({
        options: ["720p", "480p", "360p"],
        current: "original",
      });
      player.time = 30;
      player.pausedValue = false;
      player.rate = 1.5;
      act(() => player.trigger("play"));

      select(player, "480p");
      expect(mock.instances).toHaveLength(1);
      expect(player.sources).toHaveLength(2);
      expect(player.sources[1]).toMatchObject({
        src: expect.stringMatching(
          /^\/api\/videos\/7\/transcode\.mp4\?startMs=30000&attempt=[0-9a-f]{32}&quality=480p$/,
        ) as unknown,
        vvQuality: "480p",
        vvOffsetSeconds: 30,
      });
      expect(window.localStorage.getItem("vv.playback-quality.v1")).toBe(
        JSON.stringify("480p"),
      );
      expect(player.qualityStates.at(-1)).toMatchObject({ current: "480p" });
      expect(
        await screen.findByRole("button", { name: "Converting to 480p" }),
      ).toBeDefined();

      // 差し替えた要素は止まった状態・既定の速度から始まる。
      player.pausedValue = true;
      player.rate = 1;
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(false);
      expect(player.rate).toBe(1.5);
      // 変換の source は URL が位置を持つので、シークしない。
      expect(player.time).toBe(0);
    });

    it("止めた状態で選ぶと、同じ位置で止まったままにする", async () => {
      const values = props(hd);
      const player = await playerFor(values);
      player.time = 30;
      select(player, "480p");
      expect(player.sources[1]).toMatchObject({ vvOffsetSeconds: 30, vvQuality: "480p" });
      act(() => player.trigger("pause"));
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(true);
      expect(values.onPosition).toHaveBeenLastCalledWith(30_000);
      expect(values.onProgress).not.toHaveBeenCalled();
    });

    it("元の画質に戻すと直接再生に戻り、メタデータのあとで切り替えた位置へシークして続ける", async () => {
      window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
      const values = props(hd);
      const player = await playerFor(values);
      expect(player.sources[0]).toMatchObject({ vvQuality: "480p" });
      await screen.findByRole("button", { name: "Converting to 480p" });
      player.time = 40;
      player.pausedValue = false;
      act(() => player.trigger("play"));

      select(player, "original");
      expect(player.sources.at(-1)).toEqual({
        src: "/api/videos/7/stream",
        type: "video/mp4",
      });
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: /Converting/ })).toBeNull(),
      );
      player.pausedValue = true;
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(40);
      expect(player.pausedValue).toBe(false);
      expect(values.onPosition).toHaveBeenLastCalledWith(40_000);
      expect(window.localStorage.getItem("vv.playback-quality.v1")).toBe(
        JSON.stringify("original"),
      );
    });

    it("続けて2回変えると最後の画質の source だけが残り、前の切り替えのメタデータは新しい source をシークしない", async () => {
      window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
      const values = props(hd);
      const player = await playerFor(values);
      player.time = 40;
      player.pausedValue = false;
      act(() => player.trigger("play"));

      select(player, "original");
      select(player, "360p");
      expect(player.sources).toHaveLength(3);
      expect(player.sources.at(-1)).toMatchObject({
        src: expect.stringMatching(
          /startMs=40000&attempt=[0-9a-f]{32}&quality=360p$/,
        ) as unknown,
        vvQuality: "360p",
      });
      expect(player.qualityStates.at(-1)).toMatchObject({ current: "360p" });
      // 直接再生へ戻す切り替えのシークは、置き換えた変換の source には効かない。
      player.pausedValue = true;
      player.time = 0;
      act(() => player.trigger("loadedmetadata"));
      expect(player.time).toBe(0);
      expect(player.pausedValue).toBe(false);
      expect(window.localStorage.getItem("vv.playback-quality.v1")).toBe(
        JSON.stringify("360p"),
      );
    });

    it("変換の画質を続けて変えると、前の切り替えのメタデータでは速度と再生を戻さず、最後の source のメタデータで戻す", async () => {
      const values = props(hd);
      const player = await playerFor(values);
      player.time = 30;
      player.pausedValue = false;
      player.rate = 1.5;
      act(() => player.trigger("play"));

      select(player, "480p");
      select(player, "360p");
      const [, first, last] = player.sources as { src: string }[];
      if (first === undefined || last === undefined)
        throw new Error("sourceがありません");
      expect(first.src).toMatch(/quality=480p$/);
      expect(last.src).toMatch(/quality=360p$/);

      // video.js が 360p を要素へ渡す前に、480p のメタデータが届く。
      player.pausedValue = true;
      player.rate = 1;
      player.loadInTech(first.src);
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(true);
      expect(player.rate).toBe(1);

      // 360p が要素に渡ったあと、未 buffer シークで仲立ちが同じ画質の URL へ読み込み直しても、
      // そのメタデータで切り替えを終える。
      player.loadInTech(last.src);
      act(() => player.trigger("loadstart"));
      player.loadInTech(
        "/api/videos/7/transcode.mp4?startMs=50000&attempt=ab&quality=360p",
      );
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(false);
      expect(player.rate).toBe(1.5);
    });

    it("切り替えた変換の失敗は今の誤りの経路で伝わり、再試行の位置は切り替えた位置", async () => {
      const values = props(hd);
      const player = await playerFor(values);
      player.time = 25;
      player.pausedValue = false;
      act(() => player.trigger("play"));
      select(player, "480p");
      player.errorValue = { code: 3 };
      act(() => player.trigger("error"));
      expect(values.onError).toHaveBeenCalledWith(25_000, "decode");
      expect(player.sources).toHaveLength(2);
    });

    it("切り替えの読み込み中に止めたら、読み込んだあとも止めたままにする", async () => {
      const values = props(hd);
      const player = await playerFor(values);
      player.time = 30;
      player.pausedValue = false;
      act(() => player.trigger("play"));
      select(player, "480p");
      act(() => player.pause());
      act(() => player.trigger("loadedmetadata"));
      expect(player.pausedValue).toBe(true);
    });

    it("今と同じ画質を選ぶと、読み込み直さずに選んだ画質を覚える", async () => {
      const values = props(hd);
      const player = await playerFor(values);
      select(player, "original");
      expect(player.sources).toHaveLength(1);
      expect(window.localStorage.getItem("vv.playback-quality.v1")).toBe(
        JSON.stringify("original"),
      );
    });

    it("覚えた画質が使えず元の画質で再生しているとき、「元の画質」を選び直すと覚えた画質を置き換える", async () => {
      window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
      const values = props({ width: 640, height: 360 });
      const player = await playerFor(values);
      expect(player.qualityStates.at(-1)).toMatchObject({ current: "original" });

      select(player, "original");
      expect(player.sources).toHaveLength(1);
      expect(window.localStorage.getItem("vv.playback-quality.v1")).toBe(
        JSON.stringify("original"),
      );
    });
  });

  describe("回線の遅さで途切れるとき", () => {
    function lastStatus(values: ReturnType<typeof props>) {
      const calls = values.onStatus.mock.calls;
      return calls[calls.length - 1]?.[0] as Record<string, unknown> | undefined;
    }

    /** 再生を始め、最初の読み込みが落ち着いた（playing が来た）プレイヤー。 */
    function startPlaying(values: ReturnType<typeof props>) {
      vi.useFakeTimers();
      render(<VideoPlayer {...values} />);
      const player = mock.instances[0];
      if (player === undefined) throw new Error("playerがありません");
      act(() => {
        player.trigger("loadstart");
        player.trigger("loadedmetadata");
        player.pausedValue = false;
        player.trigger("play");
        player.trigger("waiting");
        player.trigger("playing");
      });
      return player;
    }

    function stallFor(player: ReturnType<typeof startPlaying>, ms: number) {
      act(() => player.trigger("waiting"));
      act(() => vi.advanceTimersByTime(ms));
      act(() => player.trigger("playing"));
    }

    it("再生中の waiting が 60 秒の中で 3 回来ると stalled を知らせ、再生は止めない", () => {
      const values = props();
      const player = startPlaying(values);
      stallFor(player, 1000);
      act(() => vi.advanceTimersByTime(20_000));
      stallFor(player, 1000);
      act(() => vi.advanceTimersByTime(20_000));
      expect(lastStatus(values)).toMatchObject({ stalled: false });
      act(() => player.trigger("waiting"));
      expect(lastStatus(values)).toMatchObject({ stalled: true, loading: true });
      expect(player.pausedValue).toBe(false);
      expect(player.sources).toHaveLength(1);
    });

    it("1 回の waiting が playing の来ないまま 10 秒続くと、その最中に stalled を知らせる", () => {
      const values = props();
      const player = startPlaying(values);
      act(() => player.trigger("waiting"));
      act(() => vi.advanceTimersByTime(9_999));
      expect(lastStatus(values)).toMatchObject({ stalled: false });
      act(() => vi.advanceTimersByTime(1));
      expect(lastStatus(values)).toMatchObject({ stalled: true, loading: true });
      expect(player.pausedValue).toBe(false);
    });

    it("シーク直後の waiting は数えない", () => {
      const values = props();
      const player = startPlaying(values);
      for (let i = 0; i < 3; i += 1) {
        act(() => player.trigger("seeking"));
        stallFor(player, 1000);
      }
      act(() => player.trigger("seeking"));
      act(() => player.trigger("waiting"));
      act(() => vi.advanceTimersByTime(15_000));
      expect(values.onStatus).not.toHaveBeenCalledWith(
        expect.objectContaining({ stalled: true }),
      );
    });

    it("再生の終わりで stalled を下ろす", () => {
      const values = props();
      const player = startPlaying(values);
      act(() => player.trigger("waiting"));
      act(() => vi.advanceTimersByTime(10_000));
      expect(lastStatus(values)).toMatchObject({ stalled: true });
      act(() => player.trigger("ended"));
      expect(lastStatus(values)).toMatchObject({ stalled: false, ended: true });
    });
  });
});
