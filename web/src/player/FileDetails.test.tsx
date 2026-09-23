import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import FileDetails from "./FileDetails";

const video: Video = {
  id: 7,
  title: "テスト動画",
  sizeBytes: 1024,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "pending",
  durationMs: 600_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
};

function labels(): string[] {
  return [...document.querySelectorAll("[data-state='active'] dt")].map(
    (node) => node.textContent ?? "",
  );
}

function values(): string[] {
  return [...document.querySelectorAll("[data-state='active'] dd")].map(
    (node) => node.textContent ?? "",
  );
}

describe("FileDetails", () => {
  it("動画情報の6項目を仕様の順序で表示する", () => {
    render(<FileDetails video={video} />);

    expect(labels().slice(0, 6)).toEqual([
      "長さ",
      "解像度",
      "形式",
      "映像",
      "音声",
      "大きさ",
    ]);
    expect(values().slice(0, 6)).toEqual([
      "10:00",
      "1920×1080",
      "mp4",
      "h264",
      "aac",
      "1.0 KB",
    ]);
  });

  it.each([
    ["pending", undefined, "確認中"],
    ["failed", "ffprobe error", "読み取れませんでした（ffprobe error）"],
    ["done", undefined, "なし"],
  ] as const)(
    "解析状態 %s の欠落値を空欄にしない",
    (probeState, probeError, fallback) => {
      render(
        <FileDetails
          video={{
            ...video,
            probeState,
            probeError,
            durationMs: undefined,
            width: undefined,
            height: undefined,
            container: undefined,
            videoCodec: undefined,
            audioCodec: undefined,
          }}
        />,
      );

      expect(values().slice(0, 5)).toEqual(Array(5).fill(fallback));
    },
  );
});
