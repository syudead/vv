import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import {
  formatBytes,
  formatDuration,
  formatRelative,
  qualityLabel,
  unplayableText,
  watchState,
  watchedRatio,
} from "./format";

const base: Video = {
  id: 1,
  title: "t",
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
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

describe("formatRelative", () => {
  const now = new Date("2026-09-20T12:00:00Z");
  it("段階的に丸める", () => {
    expect(formatRelative("2026-09-20T11:59:50Z", now)).toBe("たった今");
    expect(formatRelative("2026-09-20T11:30:00Z", now)).toBe("30 分前");
    expect(formatRelative("2026-09-20T06:00:00Z", now)).toBe("6 時間前");
    expect(formatRelative("2026-09-14T12:00:00Z", now)).toBe("6 日前");
    expect(formatRelative("2026-08-30T12:00:00Z", now)).toBe("3 週間前");
    expect(formatRelative("2026-03-20T12:00:00Z", now)).toBe("6 か月前");
    expect(formatRelative("2020-09-20T12:00:00Z", now)).toBe("6 年前");
  });
  it("壊れた日付は空", () => {
    expect(formatRelative("nope", now)).toBe("");
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
  it("理由ごとの文言", () => {
    expect(unplayableText({ ...base, playable: false, probeState: "pending" })).toBe(
      "確認中",
    );
    expect(unplayableText({ ...base, playable: false, probeState: "failed" })).toBe(
      "読み取れませんでした",
    );
    expect(
      unplayableText({
        ...base,
        playable: false,
        unplayableReason: "container",
        container: "mkv",
      }),
    ).toBe("mkv は再生できません");
    expect(
      unplayableText({
        ...base,
        playable: false,
        unplayableReason: "video_codec",
        videoCodec: "hevc",
      }),
    ).toBe("映像 hevc は再生できません");
  });
});
