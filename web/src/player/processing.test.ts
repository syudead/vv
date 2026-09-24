import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import { creatingLine, processingStages } from "./processing";

const base: Video = {
  id: 1,
  title: "a",
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
      ["ファイルの検出", "done"],
      ["動画情報の読み取り", "active"],
      ["サムネイル", "waiting"],
      ["シーク用プレビュー", "waiting"],
      ["一覧用プレビュー", "waiting"],
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
      ["ファイルの検出", "done"],
      ["動画情報の読み取り", "active"],
      ["サムネイル", "failed"],
      ["シーク用プレビュー", "done"],
      ["一覧用プレビュー", "waiting"],
    ]);
    expect(states({ thumbnailState: "failed", previewState: "pending" })).toEqual([
      ["ファイルの検出", "done"],
      ["動画情報の読み取り", "done"],
      ["サムネイル", "failed"],
      ["シーク用プレビュー", "done"],
      ["一覧用プレビュー", "active"],
    ]);
  });
});

describe("creatingLine", () => {
  it.each([
    [{ previewState: "pending" }, "一覧用プレビューを作成中 · 再生はできます"],
    [
      { seekThumbnailState: "pending", previewState: "pending" },
      "シーク用プレビューと一覧用プレビューを作成中 · 再生はできます",
    ],
    [
      {
        thumbnailState: "pending",
        seekThumbnailState: "pending",
        previewState: "pending",
      },
      "サムネイルとシーク用プレビューと一覧用プレビューを作成中 · 再生はできます",
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
