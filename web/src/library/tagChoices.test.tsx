import { describe, expect, it } from "vitest";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { buildAddOptions } from "./tagChoices";

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
