import { describe, expect, it } from "vitest";

import type { Tag, VideoTagsSummary } from "../api/tags";
import { t } from "../i18n";
import { buildAddOptions, buildRemoveOptions, removableSummary } from "./tagChoices";

function tag(id: number, name: string, synonyms: string[] = [], videoCount = 1): Tag {
  return {
    id,
    name,
    synonyms,
    videoCount,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
  };
}

function summaryItem(id: number, name: string, manualCount: number, count = manualCount) {
  return { tag: { id, name, tentative: false }, count, manualCount };
}

describe("buildAddOptions", () => {
  const tags = [tag(1, "cat", [], 3), tag(2, "scat"), tag(3, "dog", ["kitten"])];

  it("前方一致を先に、残りを自然順に並べ、同義語だけで当たった行に印を付ける", () => {
    const { options, exactOption } = buildAddOptions(tags, "  CAT ");
    expect(options.map((option) => option.label)).toEqual(["cat", "scat"]);
    expect(options[0]?.meta).toEqual(t.library.selection.videoCount(3));
    expect(exactOption).toBeNull();

    const bySynonym = buildAddOptions(tags, "kit");
    expect(bySynonym.options.map((option) => option.label)).toEqual(["dog"]);
    expect(bySynonym.options[0]?.hint).toEqual(t.library.selection.synonym("kitten"));
  });

  it("名前か同義語がそのまま一致するタグを exactOption にする", () => {
    expect(buildAddOptions(tags, " cat ").exactOption?.id).toBe("1");
    expect(buildAddOptions(tags, "kitten").exactOption?.id).toBe("3");
    expect(buildAddOptions(tags, "Cat").exactOption).toBeNull();
  });

  it("空の入力では全タグを出す", () => {
    expect(buildAddOptions(tags, "").options).toHaveLength(3);
  });
});

describe("removableSummary", () => {
  it("手で付けた分の無いタグを落とす", () => {
    const summary: VideoTagsSummary = {
      total: 2,
      items: [summaryItem(1, "a", 0, 2), summaryItem(2, "b", 1, 2)],
    };
    expect(removableSummary(summary).items.map((item) => item.tag.id)).toEqual([2]);
    expect(removableSummary(summary).total).toBe(2);
  });
});

describe("buildRemoveOptions", () => {
  const summary: VideoTagsSummary = {
    total: 2,
    items: [summaryItem(1, "scat", 2), summaryItem(2, "cat", 1)],
  };

  it("前方一致を先に並べ、一部にだけ付いたタグに aria-label を付ける", () => {
    const { options, exactOption } = buildRemoveOptions(summary, "cat");
    expect(options.map((option) => option.label)).toEqual(["cat", "scat"]);
    expect(options[0]?.ariaLabel).toEqual(t.library.selection.partialLabel("cat", 1, 2));
    expect(options[1]?.ariaLabel).toBeUndefined();
    expect(options[1]?.meta).toEqual(t.library.selection.videoCount(2));
    expect(exactOption?.id).toBe("2");
  });

  it("名前がそのまま一致しなければ exactOption は null", () => {
    expect(buildRemoveOptions(summary, "ca").exactOption).toBeNull();
  });
});
