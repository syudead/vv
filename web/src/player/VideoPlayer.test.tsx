import { render, waitFor } from "@testing-library/react";
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

    on(event: string, callback: Callback) {
      this.handlers.set(event, [...(this.handlers.get(event) ?? []), callback]);
    }
    ready(callback: Callback) {
      callback();
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
    vi.fn(() => {
      const player = new FakePlayer();
      instances.push(player);
      return player;
    }),
    {
      use: vi.fn(),
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
  sizeBytes: 100,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  durationMs: 120_000,
  videoCodec: "h264",
  container: "mp4",
};

function props(overrides: Partial<Video> = {}) {
  return {
    video: { ...video, ...overrides },
    initialPositionMs: 0,
    onPosition: vi.fn(),
    onProgress: vi.fn(),
    onResumed: vi.fn(),
    onError: vi.fn(),
  };
}

describe("VideoPlayer", () => {
  afterEach(() => {
    mock.instances.length = 0;
    vi.clearAllMocks();
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
});
