import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import type { ListCriteria } from "./listCriteria";
import { conditionLabels, summarize } from "./listSummary";

function criteria(extra: Partial<ListCriteria> = {}): ListCriteria {
  return { query: "", watch: "all", playable: false, sort: "addedDesc", ...extra };
}

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    sizeBytes: 1024 * 1024,
    addedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    durationMs: 60_000,
    previewState: "done",
    ...extra,
  };
}

describe("conditionLabels", () => {
  it("lists nothing when no condition applies", () => {
    expect(conditionLabels(criteria({ sort: "titleAsc" }))).toEqual([]);
  });

  it("names the query, the watch state and playable-only in order", () => {
    expect(
      conditionLabels(criteria({ query: "京都", watch: "unwatched", playable: true })),
    ).toEqual(["検索語「京都」", "未視聴", "再生できるものだけ"]);
  });
});

describe("summarize", () => {
  it("reports zero items", () => {
    expect(summarize([], 0)).toBe("0 件（0 B）");
  });

  it("reports every item with total duration and size when all are loaded", () => {
    expect(summarize([video(1), video(2)], 2)).toBe("2 件（2:00 · 2.0 MB）");
  });

  it("reports the loaded range against the server total", () => {
    expect(summarize([video(1, { durationMs: undefined })], 1200)).toBe(
      "1–1 / 1,200 件（1.0 MB）",
    );
  });
});
