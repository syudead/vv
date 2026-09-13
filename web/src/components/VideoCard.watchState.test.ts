import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import { partialRatio, unplayableText } from "./VideoCard";

/**
 * 視聴状況の読み替え（data-model.md 3.）。
 *
 * 帯と印はサーバーが返す Progress から導くもので、新しく受け取る項目ではない。
 * 導出の規則をここで固定しておくと、見た目を書き換えても意味が変わらない
 * （FR-025 / SC-009）。
 */

/** video は検査に要る項目だけを差し替えられる Video を作る。 */
function video(overrides: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "題名",
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    ...overrides,
  };
}

describe("partialRatio（途中まで見た割合）", () => {
  it("見ていない動画は帯を描かない", () => {
    expect(partialRatio(video())).toBeNull();
  });

  it("見終わった動画は帯を描かない", () => {
    const watched = video({
      durationMs: 100_000,
      progress: {
        positionMs: 50_000,
        completed: true,
        updatedAt: "2026-09-13T00:00:00Z",
      },
    });
    expect(partialRatio(watched)).toBeNull();
  });

  it("途中まで見た動画は割合を返す", () => {
    const partial = video({
      durationMs: 100_000,
      progress: {
        positionMs: 25_000,
        completed: false,
        updatedAt: "2026-09-13T00:00:00Z",
      },
    });
    expect(partialRatio(partial)).toBeCloseTo(0.25);
  });

  // 尺が取れていない動画で割合を出すと、帯の長さが嘘になる。
  // 「未視聴と同じ見せ方にする」のが data-model.md 3. の規則である。
  it("尺が取れていない動画は、位置が正でも帯を描かない", () => {
    const unknownDuration = video({
      durationMs: undefined,
      probeState: "pending",
      progress: {
        positionMs: 25_000,
        completed: false,
        updatedAt: "2026-09-13T00:00:00Z",
      },
    });
    expect(partialRatio(unknownDuration)).toBeNull();
  });

  it("尺が 0 以下の動画は帯を描かない", () => {
    for (const durationMs of [0, -1]) {
      const broken = video({
        durationMs,
        progress: {
          positionMs: 25_000,
          completed: false,
          updatedAt: "2026-09-13T00:00:00Z",
        },
      });
      expect(partialRatio(broken)).toBeNull();
    }
  });

  it("位置が 0 以下なら帯を描かない", () => {
    const notStarted = video({
      durationMs: 100_000,
      progress: { positionMs: 0, completed: false, updatedAt: "2026-09-13T00:00:00Z" },
    });
    expect(partialRatio(notStarted)).toBeNull();
  });

  it("割合が 1 を超えない", () => {
    const overrun = video({
      durationMs: 100_000,
      progress: {
        positionMs: 200_000,
        completed: false,
        updatedAt: "2026-09-13T00:00:00Z",
      },
    });
    expect(partialRatio(overrun)).toBe(1);
  });
});

describe("unplayableText（再生できない理由）", () => {
  it("再生できる動画は何も出さない", () => {
    expect(unplayableText(video())).toBeNull();
  });

  // 解析が終わっていない・失敗した状態の unplayableReason は当てにならない。
  // 判定できていないことを、判定結果より先に伝える。
  it("解析中は unplayableReason より「確認中」が優先される", () => {
    const pending = video({
      playable: false,
      probeState: "pending",
      unplayableReason: "container",
      container: "mkv",
    });
    expect(unplayableText(pending)).toBe("確認中");
  });

  it("解析に失敗したときは unplayableReason より「読み取れませんでした」が優先される", () => {
    const failed = video({
      playable: false,
      probeState: "failed",
      unplayableReason: "video_codec",
      videoCodec: "hevc",
    });
    expect(unplayableText(failed)).toBe("読み取れませんでした");
  });

  it("解析が済んでいれば理由ごとに言い分ける", () => {
    expect(
      unplayableText(
        video({ playable: false, unplayableReason: "container", container: "mkv" }),
      ),
    ).toBe("mkv は再生できません");
    expect(
      unplayableText(
        video({ playable: false, unplayableReason: "video_codec", videoCodec: "hevc" }),
      ),
    ).toBe("映像の形式 (hevc) は再生できません");
    expect(
      unplayableText(
        video({ playable: false, unplayableReason: "audio_codec", audioCodec: "ac3" }),
      ),
    ).toBe("音声の形式 (ac3) は再生できません");
  });

  it("理由が分からないときも空欄にしない", () => {
    expect(unplayableText(video({ playable: false }))).toBe("再生できません");
  });
});
