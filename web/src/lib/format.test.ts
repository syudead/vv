import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import {
  formatBytes,
  formatDuration,
  isNarrowVideo,
  qualityLabel,
  unplayableText,
  watchState,
  watchedRatio,
} from "./format";

const base: Video = {
  id: 1,
  title: "t",
  public: false,
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "pending",
  durationMs: 10_000,
  videoCodec: "h264",
  tags: [],
};

describe("formatDuration", () => {
  it("m:ss と h:mm:ss", () => {
    expect(formatDuration(59_000)).toBe("0:59");
    expect(formatDuration(3_599_000)).toBe("59:59");
    expect(formatDuration(3_600_000)).toBe("1:00:00");
  });
  it("不明なら空", () => {
    expect(formatDuration(undefined)).toBe("");
    expect(formatDuration(-1)).toBe("");
  });
});

describe("formatBytes", () => {
  it("単位を選ぶ", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(84_331_821)).toBe("80.4 MB");
    expect(formatBytes(2.4 * 1024 ** 3)).toBe("2.4 GB");
  });
});

describe("qualityLabel", () => {
  it("短辺で決める", () => {
    expect(qualityLabel({ width: 1920, height: 1080 })).toBe("1080p");
    expect(qualityLabel({ width: 1080, height: 1920 })).toBe("1080p");
    expect(qualityLabel({ width: 3840, height: 2160 })).toBe("4K");
    expect(qualityLabel({ width: 640, height: 360 })).toBe("360p");
    expect(qualityLabel({})).toBe("");
  });
});

describe("watchState / watchedRatio", () => {
  const at = "2026-09-20T00:00:00Z";
  it("3 値に畳む", () => {
    expect(watchState(base)).toBe("unwatched");
    expect(
      watchState({ progress: { positionMs: 0, completed: false, updatedAt: at } }),
    ).toBe("unwatched");
    expect(
      watchState({ progress: { positionMs: 10, completed: false, updatedAt: at } }),
    ).toBe("inProgress");
    expect(
      watchState({ progress: { positionMs: 10, completed: true, updatedAt: at } }),
    ).toBe("watched");
  });
  it("途中のときだけ割合を返す", () => {
    expect(
      watchedRatio({
        durationMs: 100,
        progress: { positionMs: 25, completed: false, updatedAt: at },
      }),
    ).toBe(0.25);
    expect(
      watchedRatio({
        durationMs: 100,
        progress: { positionMs: 25, completed: true, updatedAt: at },
      }),
    ).toBeNull();
    expect(
      watchedRatio({ progress: { positionMs: 25, completed: false, updatedAt: at } }),
    ).toBeNull();
  });
});

describe("unplayableText", () => {
  it("再生できれば null", () => {
    expect(unplayableText(base)).toBeNull();
  });
  it("解析待ちと解析失敗だけ文言を返す", () => {
    expect(unplayableText({ ...base, playable: false, probeState: "pending" })).toBe(
      "Checking…",
    );
    expect(unplayableText({ ...base, playable: false, probeState: "failed" })).toBe(
      "Couldn't read this video",
    );
    expect(
      unplayableText({
        ...base,
        playable: false,
        unplayableReason: "container",
        container: "mkv",
      }),
    ).toBeNull();
    expect(unplayableText({ ...base, playable: false, durationMs: undefined })).toBe(
      "Missing information needed for playback",
    );
    expect(unplayableText({ ...base, playable: false, videoCodec: undefined })).toBe(
      "Missing information needed for playback",
    );
  });
});

describe("isNarrowVideo", () => {
  it("縦長と正方形に近い動画だけを真にする", () => {
    expect(isNarrowVideo({ width: 1080, height: 1920 })).toBe(true);
    expect(isNarrowVideo({ width: 720, height: 720 })).toBe(true);
    expect(isNarrowVideo({ width: 1920, height: 1080 })).toBe(false);
    expect(isNarrowVideo({ width: 640, height: 480 })).toBe(false);
  });

  it("表示の比率があればそれを優先する", () => {
    expect(isNarrowVideo({ width: 1920, height: 1080, displayAspectRatio: 9 / 16 })).toBe(
      true,
    );
  });

  it("寸法が分からなければ偽", () => {
    expect(isNarrowVideo({})).toBe(false);
    expect(isNarrowVideo({ width: 1080, height: 0 })).toBe(false);
  });
});
