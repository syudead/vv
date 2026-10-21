import { describe, expect, it } from "vitest";

import type { VersionJudgeEvidence } from "../api/client";
import { evidenceSentences } from "./evidenceSentences";

function evidence(extra: Partial<VersionJudgeEvidence>): VersionJudgeEvidence {
  return { rule: "frames_differ", compared: 40, differing: 6, ...extra };
}

describe("evidenceSentences", () => {
  it("ルールで同じと決めた組は AI に聞いていないと書く", () => {
    expect(
      evidenceSentences(evidence({ rule: "frames_match", differing: 1 }), undefined),
    ).toEqual([
      "1 of 40 frames compared by time differ, no two in a row and within the 5% allowed, so the rule judged them the same video without asking the AI.",
    ]);
  });

  it("比べられなかった組は、版の違いかコマ不足かを書き分ける", () => {
    expect(
      evidenceSentences(
        evidence({ rule: "incomparable", compared: 0, versionMismatch: true }),
        undefined,
      )[0],
    ).toContain("different versions of vv");
    expect(
      evidenceSentences(evidence({ rule: "incomparable", compared: 2 }), undefined),
    ).toEqual([
      "Only 2 frames could be compared by time, fewer than the 3 needed, often because the picture is flat or dark.",
    ]);
  });

  it("散らばった違いで AI の答えがまだ無いときは、見せるコマだと書く", () => {
    expect(
      evidenceSentences(
        evidence({ frames: [{ aMs: 0, bMs: 0, distance: 20 }] }),
        undefined,
      ),
    ).toEqual([
      "6 of 40 frames compared by time differ here and there, more than the 5% allowed.",
      "The AI was shown 1 pair of those frames, below, and asked whether both sides show the same picture.",
      "The AI hasn't answered yet, so these frames are what it will be shown.",
    ]);
  });

  it("同じと判定しても、違うと記録した組とぶつかったときはその理由を足す", () => {
    const sentences = evidenceSentences(
      evidence({ rule: "frames_match", differing: 0, conflict: true }),
      undefined,
    );
    expect(sentences).toHaveLength(2);
    expect(sentences[1]).toContain("already marked different");
  });

  it("開始のずれを見つけた組は、ずらして比べたことを最初に書く", () => {
    expect(
      evidenceSentences(
        evidence({ rule: "frames_match", differing: 0, offsetMs: -1000 }),
        undefined,
      )[0],
    ).toBe(
      "The right video shows the same moment 1 second earlier than the left, so their frames were compared with that shift.",
    );
  });
});
