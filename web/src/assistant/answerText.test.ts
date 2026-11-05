import { describe, expect, it } from "vitest";

import type { AssistantProgressItem } from "../api/assistant";
import { answerBlocks, maxMentioned, mentionedLabels } from "./answerText";

function item(labels: AssistantProgressItem["labels"]): AssistantProgressItem {
  return { tool: "listVideos", kind: "read", labels, at: "2026-10-30T00:00:00Z" };
}

describe("answerBlocks", () => {
  it("- と 1. の行だけをリストにし、** を落とし、ほかの行は改行ごと残す", () => {
    expect(
      answerBlocks(
        "**3 本**あります。\n次のとおり:\n- 夏\n- 海\n\n1. 山\n2. 川\n[link](x)",
      ),
    ).toEqual([
      { kind: "text", text: "3 本あります。\n次のとおり:" },
      { kind: "list", ordered: false, items: ["夏", "海"] },
      { kind: "list", ordered: true, items: ["山", "川"] },
      { kind: "text", text: "[link](x)" },
    ]);
  });
});

describe("mentionedLabels", () => {
  it("答えに語として出るラベルを答えの順に、id ごとに 1 度だけ返す", () => {
    const progress = [
      item([
        { kind: "tag", id: 1, name: "ライブ" },
        { kind: "tag", id: 2, name: "旅行" },
        { kind: "person", id: 3, name: "Alice" },
        { kind: "tagKind", name: "ジャンル" },
      ]),
      item([{ kind: "tag", id: 2, name: "旅行" }]),
    ];

    expect(
      mentionedLabels("ライブラリには ALICE と旅行の動画があります。ジャンル", progress),
    ).toEqual([
      { kind: "person", id: 3, name: "Alice" },
      { kind: "tag", id: 2, name: "旅行" },
    ]);
  });

  it(`多くても ${String(maxMentioned)} 件までにする`, () => {
    const labels = Array.from({ length: 25 }, (_, index) => ({
      kind: "tag" as const,
      id: index,
      name: `t${String(index)}`,
    }));
    const answer = labels.map((label) => label.name).join(" ");

    expect(mentionedLabels(answer, [item(labels)])).toHaveLength(maxMentioned);
  });
});
