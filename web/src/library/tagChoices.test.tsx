import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import type { TagKind } from "../api/tagKinds";
import { buildAddOptions, buildTagAndKindOptions, pickOfChoice } from "./tagChoices";

function tag(id: number, name: string, synonyms: string[] = [], videoCount = 1): Tag {
  return {
    kind: null,
    id,
    name,
    synonyms,
    videoCount,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
  };
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

function kind(id: number, name: string): TagKind {
  return { id, name, cardFirst: false, tagCount: 1, createdAt: "2026-01-01T00:00:00Z" };
}

describe("buildTagAndKindOptions", () => {
  const tags = [tag(1, "cat"), tag(2, "scat"), tag(3, "ねこ")];
  const kinds = [kind(5, "Category"), kind(6, "Place")];

  it("ranks tags and kinds together by folded prefix, then natural name, and marks kinds", () => {
    const { options } = buildTagAndKindOptions(tags, kinds, "CAT");
    expect(options.map((option) => option.label)).toEqual(["cat", "Category", "scat"]);
    expect(options[1]?.ariaLabel).toBe("Tag kind Category");
    expect(options.map(pickOfChoice)).toEqual([
      { kind: "tag", id: 1 },
      { kind: "tagKind", id: 5 },
      { kind: "tag", id: 2 },
    ]);
    expect(buildTagAndKindOptions(tags, kinds, "ネコ").options[0]?.label).toBe("ねこ");
  });

  it("an exact tag spelling wins over a kind of the same name", () => {
    const sameName = [kind(7, "cat")];
    expect(buildTagAndKindOptions(tags, sameName, "cat").exactOption?.id).toBe("1");
    const kindOnly = buildTagAndKindOptions([], sameName, "cat").exactOption;
    expect(kindOnly === null ? null : pickOfChoice(kindOnly)).toEqual({
      kind: "tagKind",
      id: 7,
    });
  });
});
