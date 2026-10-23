import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import { formatCodec, technicalSummary } from "./properties";

const video: Video = {
  id: 7,
  virtual: false,
  title: "a",
  sizeBytes: 1,
  addedAt: "2026-09-20T03:00:00Z",
  updatedAt: "2026-09-20T03:00:00Z",
  fileCreatedAt: "2026-09-20T03:00:00Z",
  fileModifiedAt: "2026-09-20T03:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  tags: [],
  public: false,
};

describe("property helpers", () => {
  it("コーデックを大文字の表記にする", () => {
    expect(formatCodec("h264")).toBe("H.264");
    expect(formatCodec("hevc")).toBe("H.265");
    expect(formatCodec("vp9")).toBe("VP9");
    expect(formatCodec("aac")).toBe("AAC");
  });

  it("技術情報の状態を返す", () => {
    expect(technicalSummary({ ...video, probeState: "pending" })).toEqual({
      kind: "pending",
    });
    expect(technicalSummary({ ...video, probeState: "failed" })).toEqual({
      kind: "failed",
    });
  });
});
