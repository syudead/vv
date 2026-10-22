import { describe, expect, it } from "vitest";

import type { VideoTag } from "./client";
import {
  applyTagToTags,
  cardTagOrder,
  compareNatural,
  compareTagRefs,
  tagsReflectChange,
} from "./tagOrder";

describe("cardTagOrder", () => {
  const ref = (id: number, name: string, kind: { name: string; cardFirst: boolean } | null) => ({
    id,
    name,
    tentative: false,
    kind: kind === null ? null : { id: kind.name.length, ...kind },
  });

  it("先頭に出す種類のタグを種類名の自然順で前に寄せ、それ以外は元の順を保つ", () => {
    const place = { name: "Place", cardFirst: true };
    const genre = { name: "Genre", cardFirst: true };
    const work = { name: "Work", cardFirst: false };
    const tags = [
      ref(1, "a", null),
      ref(2, "b", place),
      ref(3, "c", work),
      ref(4, "d", genre),
      ref(5, "e", genre),
    ];
    expect(cardTagOrder(tags).map((tag) => tag.id)).toEqual([4, 5, 2, 1, 3]);
  });
});

describe("compareNatural", () => {
  it("数字の連続を数値として比べる（2話 < 10話）", () => {
    expect(compareNatural("2話", "10話")).toBeLessThan(0);
    expect(compareNatural("10話", "2話")).toBeGreaterThan(0);
  });

  it("先頭のゼロは値の比較に影響しない", () => {
    expect(compareNatural("話01", "話1")).toBe(0);
  });

  it("数字以外は大文字小文字を区別せずに比べる", () => {
    expect(compareNatural("Anime", "anime")).toBe(0);
    expect(compareNatural("abc", "abd")).toBeLessThan(0);
  });

  it("全角と半角は別の文字として扱う", () => {
    expect(compareNatural("Ａ", "A")).not.toBe(0);
  });

  it("片方が他方の接頭辞なら短い方が先", () => {
    expect(compareNatural("旅行", "旅行記")).toBeLessThan(0);
  });

  it("等しい文字列は0", () => {
    expect(compareNatural("旅行", "旅行")).toBe(0);
  });
});

describe("compareTagRefs", () => {
  it("名前の自然順で比べ、同名はidで決着させる", () => {
    expect(
      compareTagRefs(
        { kind: null, id: 1, name: "2話", tentative: false },
        { kind: null, id: 2, name: "10話", tentative: false },
      ),
    ).toBeLessThan(0);
    expect(
      compareTagRefs(
        { kind: null, id: 2, name: "旅行", tentative: false },
        { kind: null, id: 1, name: "旅行", tentative: false },
      ),
    ).toBeGreaterThan(0);
  });
});

describe("applyTagToTags", () => {
  const manual = (id: number, name: string): VideoTag => ({
    kind: null,
    id,
    name,
    manual: true,
    fromFolder: false,
    tentative: false,
  });
  const folder = (id: number, name: string): VideoTag => ({
    kind: null,
    id,
    name,
    manual: false,
    fromFolder: true,
    tentative: false,
  });

  it("addは名前の自然順を保つ位置へ挿す", () => {
    const tags = [manual(1, "2話"), manual(3, "10話")];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 2, name: "5話", tentative: false },
      "add",
    );
    expect(result).toEqual([manual(1, "2話"), manual(2, "5話"), manual(3, "10話")]);
    expect(tags).toHaveLength(2);
  });

  it("addは既に付いていた同じidの行を、最新のnameで差し替える（N6）", () => {
    const tags = [manual(1, "Banana"), manual(2, "Cherry")];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 1, name: "Date", tentative: false },
      "add",
    );
    expect(result).toEqual([manual(2, "Cherry"), manual(1, "Date")]);
  });

  it("addで同じidかつ同じnameなら変えない（新しい配列でも中身は同じ）", () => {
    const tags = [manual(1, "旅行")];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 1, name: "旅行", tentative: false },
      "add",
    );
    expect(result).toEqual(tags);
    expect(result).not.toBe(tags);
  });

  it("addはフォルダ名から付いている行に手で付けた分を足し、出所を両方持つ", () => {
    const result = applyTagToTags(
      [folder(1, "旅行")],
      { kind: null, id: 1, name: "旅行", tentative: false },
      "add",
    );
    expect(result).toEqual([
      {
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: true,
        tentative: false,
        kind: null,
      },
    ]);
  });

  it("removeは同じidの行を取り除く", () => {
    const tags = [manual(1, "旅行"), manual(2, "観光")];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 1, name: "旅行", tentative: false },
      "remove",
    );
    expect(result).toEqual([manual(2, "観光")]);
  });

  it("removeはフォルダ名からも付いている行を残し、手で付けた分だけを外す", () => {
    const tags: VideoTag[] = [
      {
        kind: null,
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: true,
        tentative: false,
      },
    ];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 1, name: "旅行", tentative: false },
      "remove",
    );
    expect(result).toEqual([folder(1, "旅行")]);
  });

  it("removeでフォルダ名からの分が残る行は、返ったnameとtentativeを映す", () => {
    const tags: VideoTag[] = [
      {
        kind: null,
        id: 1,
        name: "旅行",
        manual: true,
        fromFolder: true,
        tentative: true,
      },
      folder(2, "観光"),
    ];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 1, name: "温泉", tentative: false },
      "remove",
    );
    expect(result).toEqual([folder(1, "温泉"), folder(2, "観光")]);
  });

  it("removeで無いidを渡しても変えない", () => {
    const tags = [manual(1, "旅行")];
    const result = applyTagToTags(
      tags,
      { kind: null, id: 99, name: "無関係", tentative: false },
      "remove",
    );
    expect(result).toEqual(tags);
    expect(result).not.toBe(tags);
  });
});

describe("tagsReflectChange", () => {
  const folderOnly: VideoTag = {
    kind: null,
    id: 1,
    name: "Anime",
    manual: false,
    fromFolder: true,
    tentative: false,
  };
  const both: VideoTag = {
    kind: null,
    id: 1,
    name: "Anime",
    manual: true,
    fromFolder: true,
    tentative: false,
  };

  it("フォルダ名からだけ付いている行は、手で付けた結果をまだ映していない", () => {
    expect(tagsReflectChange([folderOnly], 1, "add")).toBe(false);
    expect(tagsReflectChange([both], 1, "add")).toBe(true);
  });

  it("手で外した結果は、フォルダ名からの行が残っていても映している", () => {
    expect(tagsReflectChange([both], 1, "remove")).toBe(false);
    expect(tagsReflectChange([folderOnly], 1, "remove")).toBe(true);
    expect(tagsReflectChange([], 1, "remove")).toBe(true);
  });
});
