import { describe, expect, it } from "vitest";

import { compareNatural } from "../api/tagOrder";
import type { Tag, VideoTagsSummary } from "../api/tags";
import { t } from "../i18n";
import type { ComboboxOption } from "../ui/Combobox";
import {
  buildAddOptions,
  buildRemoveOptions,
  buildTagChoices,
  removableSummary,
  type TagChoiceText,
} from "./tagChoices";

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

/**
 * referenceChoices は索引を使う前の実装（選択バーの buildAddOptions と再生画面の
 * buildOptions）の写しである。打鍵のたびに絞り込みと並べ替えをやり直す。新しい
 * 実装の結果がこれと同じであることを確かめる（issue 675）。
 */
function referenceChoices(
  allTags: readonly Tag[],
  excludedIds: ReadonlySet<number>,
  input: string,
  text: TagChoiceText,
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const strings = text === "selection" ? t.library.selection : t.player.tags;
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();

  let exactTag: Tag | undefined;
  for (const tag of allTags) {
    if (tag.name === trimmed || tag.synonyms.includes(trimmed)) {
      exactTag = tag;
      break;
    }
  }

  const matched = allTags
    .filter((tag) => !excludedIds.has(tag.id))
    .map((tag) => {
      const nameMatch = query === "" || tag.name.toLowerCase().includes(query);
      const synonymHit = tag.synonyms.find((synonym) =>
        synonym.toLowerCase().includes(query),
      );
      if (!nameMatch && synonymHit === undefined) return null;
      const namePrefix = query === "" || tag.name.toLowerCase().startsWith(query);
      const synonymPrefix =
        synonymHit !== undefined && synonymHit.toLowerCase().startsWith(query);
      return {
        tag,
        prefix: namePrefix || synonymPrefix,
        hint:
          !nameMatch && synonymHit !== undefined
            ? strings.synonym(synonymHit)
            : undefined,
      };
    })
    .filter((value): value is NonNullable<typeof value> => value !== null)
    .sort((a, b) => {
      if (a.prefix !== b.prefix) return a.prefix ? -1 : 1;
      return compareNatural(a.tag.name, b.tag.name);
    });

  return {
    options: matched.map(({ tag, hint }) => ({
      id: String(tag.id),
      label: tag.name,
      hint,
      meta: strings.videoCount(tag.videoCount),
    })),
    exactOption:
      exactTag === undefined
        ? null
        : {
            id: String(exactTag.id),
            label: exactTag.name,
            meta: strings.videoCount(exactTag.videoCount),
          },
  };
}

describe("buildTagChoices は前の実装と同じ候補を同じ順で返す", () => {
  // 決まった種から作る疑似乱数（実行ごとに同じ一覧になる）。
  function random(seed: number): () => number {
    let state = seed;
    return () => {
      state = (state * 1103515245 + 12345) % 2147483648;
      return state / 2147483648;
    };
  }

  const fixed: Tag[] = [
    tag(1, "a10"),
    tag(2, "a2"),
    tag(3, "A1"),
    tag(4, "anime", ["アニメ", "Animation"]),
    tag(5, "Cat", ["kitten", "neko"]),
    tag(6, "scat"),
    tag(7, "dog", ["Kit", "kitten"]),
    tag(8, "cat"),
    tag(9, "b", ["cat"]),
    tag(10, "Zebra", ["ab", "xab"]),
    tag(11, "ab", ["zebra"]),
    tag(12, "a02"),
    tag(13, "item 9"),
    tag(14, "item 10"),
    tag(15, "Item 9b"),
  ];

  const next = random(675);
  const alphabet = ["a", "b", "A", "B", "c", "1", "2", "10", "0", " ", "-", "é", "猫"];
  function word(): string {
    const length = 1 + Math.floor(next() * 5);
    let result = "";
    for (let i = 0; i < length; i++) {
      result += alphabet[Math.floor(next() * alphabet.length)];
    }
    return result.trim() === "" ? "x" : result;
  }
  const generated: Tag[] = Array.from({ length: 200 }, (_, index) =>
    tag(
      100 + index,
      word(),
      Array.from({ length: Math.floor(next() * 3) }, word),
      Math.floor(next() * 5),
    ),
  );
  const allTags = [...fixed, ...generated];

  const inputs = [
    "",
    "   ",
    "a",
    "A",
    " a ",
    "a1",
    "a2",
    "a10",
    "1",
    "0",
    "b",
    "ab",
    "AB",
    "cat",
    "Cat",
    " cat ",
    "at",
    "kit",
    "kitten",
    "KITTEN",
    "neko",
    "アニメ",
    "anim",
    "zebra",
    "item",
    "item 9",
    "é",
    "猫",
    "missing",
    ...Array.from({ length: 60 }, word),
  ];
  const exclusions: ReadonlySet<number>[] = [
    new Set(),
    new Set([1, 5, 8, 11]),
    new Set(allTags.filter((_, index) => index % 3 === 0).map((item) => item.id)),
  ];

  it.each(["selection", "player"] as const)("%s の文言で", (text) => {
    for (const excludedIds of exclusions) {
      for (const input of inputs) {
        const expected = referenceChoices(allTags, excludedIds, input, text);
        const actual = buildTagChoices(allTags, input, { text, excludedIds });
        expect(
          actual,
          JSON.stringify({ input, text, excluded: [...excludedIds] }),
        ).toEqual(expected);
      }
    }
  });

  it("除く id を渡さなければ全タグから選ぶ（buildAddOptions は選択バーの文言）", () => {
    for (const input of inputs) {
      expect(buildAddOptions(allTags, input)).toEqual(
        referenceChoices(allTags, new Set(), input, "selection"),
      );
    }
  });

  it("綴りがそのまま一致するタグは、除く id に入っていても exactOption にする", () => {
    const result = buildTagChoices(allTags, "kitten", {
      text: "player",
      excludedIds: new Set([5]),
    });
    expect(result.exactOption?.id).toBe("5");
    expect(result.options.map((option) => option.id)).not.toContain("5");
  });

  it("一覧が替われば索引を作り直す", () => {
    const first = [tag(1, "b"), tag(2, "a")];
    expect(buildAddOptions(first, "").options.map((option) => option.label)).toEqual([
      "a",
      "b",
    ]);
    const second = [...first, tag(3, "a1")];
    expect(buildAddOptions(second, "a").options.map((option) => option.label)).toEqual([
      "a",
      "a1",
    ]);
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
