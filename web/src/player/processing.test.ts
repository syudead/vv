import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import { creatingLine, processingStages } from "./processing";

const base: Video = {
  id: 1,
  title: "a",
  public: false,
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  seekThumbnailState: "done",
  tags: [],
};

function states(video: Partial<Video>) {
  return processingStages({ ...base, ...video }).map((stage) => [
    stage.name,
    stage.state,
  ]);
}

describe("processingStages", () => {
  it("読み取り前は検出だけが完了で、先頭の未完了だけが処理中になる", () => {
    expect(
      states({
        probeState: "pending",
        thumbnailState: "pending",
        previewState: "pending",
        seekThumbnailState: undefined,
      }),
    ).toEqual([
      ["Finding the file", "done"],
      ["Reading video information", "active"],
      ["Thumbnail", "waiting"],
      ["Seek preview", "waiting"],
      ["List preview", "waiting"],
    ]);
  });

  it("完了と作成できなかった段を飛ばして、次の未完了を処理中にする", () => {
    expect(
      states({
        probeState: "pending",
        thumbnailState: "failed",
        seekThumbnailState: "done",
        previewState: "pending",
      }),
    ).toEqual([
      ["Finding the file", "done"],
      ["Reading video information", "active"],
      ["Thumbnail", "failed"],
      ["Seek preview", "done"],
      ["List preview", "waiting"],
    ]);
    expect(states({ thumbnailState: "failed", previewState: "pending" })).toEqual([
      ["Finding the file", "done"],
      ["Reading video information", "done"],
      ["Thumbnail", "failed"],
      ["Seek preview", "done"],
      ["List preview", "active"],
    ]);
  });
});

describe("creatingLine", () => {
  it.each([
    [
      { previewState: "pending" },
      "Creating the list preview · You can play the video now",
    ],
    [
      { seekThumbnailState: "pending", previewState: "pending" },
      "Creating the seek preview and the list preview · You can play the video now",
    ],
    [
      {
        thumbnailState: "pending",
        seekThumbnailState: "pending",
        previewState: "pending",
      },
      "Creating the thumbnail, the seek preview, and the list preview · You can play the video now",
    ],
    [{}, null],
    // failed だけが残ったら消す。
    [
      { thumbnailState: "failed", seekThumbnailState: "failed", previewState: "failed" },
      null,
    ],
    // 読み取り前・読み取り失敗では出さない（段階表示・読み取り失敗の表示が出る）。
    [{ probeState: "pending", previewState: "pending" }, null],
    [{ probeState: "failed", previewState: "pending" }, null],
  ] as [Partial<Video>, string | null][])("%j", (overrides, expected) => {
    expect(creatingLine({ ...base, ...overrides })).toBe(expected);
  });
});
