import { describe, expect, it } from "vitest";

import { applyTagToTags, compareNatural, compareTagRefs } from "./tagOrder";

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
    expect(compareTagRefs({ id: 1, name: "2話" }, { id: 2, name: "10話" })).toBeLessThan(
      0,
    );
    expect(
      compareTagRefs({ id: 2, name: "旅行" }, { id: 1, name: "旅行" }),
    ).toBeGreaterThan(0);
  });
});

describe("applyTagToTags", () => {
  it("addは名前の自然順を保つ位置へ挿す", () => {
    const tags = [
      { id: 1, name: "2話" },
      { id: 3, name: "10話" },
    ];
    const result = applyTagToTags(tags, { id: 2, name: "5話" }, "add");
    expect(result).toEqual([
      { id: 1, name: "2話" },
      { id: 2, name: "5話" },
      { id: 3, name: "10話" },
    ]);
    // 元の配列は変えない。
    expect(tags).toHaveLength(2);
  });

  it("addは既に付いていた同じidの行を、最新のnameで差し替える（N6）", () => {
    const tags = [
      { id: 1, name: "Banana" },
      { id: 2, name: "Cherry" },
    ];
    const result = applyTagToTags(tags, { id: 1, name: "Date" }, "add");
    // 改名で並びが変わる（"Date" は "Cherry" より後）ことも、この差し替えは
    // 正しく反映する。
    expect(result).toEqual([
      { id: 2, name: "Cherry" },
      { id: 1, name: "Date" },
    ]);
  });

  it("addで同じidかつ同じnameなら変えない（新しい配列でも中身は同じ）", () => {
    const tags = [{ id: 1, name: "旅行" }];
    const result = applyTagToTags(tags, { id: 1, name: "旅行" }, "add");
    expect(result).toEqual(tags);
    expect(result).not.toBe(tags);
  });

  it("removeは同じidの行を取り除く", () => {
    const tags = [
      { id: 1, name: "旅行" },
      { id: 2, name: "観光" },
    ];
    const result = applyTagToTags(tags, { id: 1, name: "旅行" }, "remove");
    expect(result).toEqual([{ id: 2, name: "観光" }]);
  });

  it("removeで無いidを渡しても変えない", () => {
    const tags = [{ id: 1, name: "旅行" }];
    const result = applyTagToTags(tags, { id: 99, name: "無関係" }, "remove");
    expect(result).toEqual(tags);
    expect(result).not.toBe(tags);
  });
});
