import { describe, expect, it } from "vitest";

import {
  hasHistoryConditions,
  parseHistoryCriteria,
  parseHistoryDate,
  serializeHistoryCriteria,
} from "./historyCriteria";

// 視聴履歴の画面の URL（specs/043-watch-history/contracts/screen-api.md「Client use」、R-12）。

function parse(search: string) {
  return parseHistoryCriteria(new URLSearchParams(search));
}

describe("parseHistoryCriteria", () => {
  it("watch・q・date を読む", () => {
    expect(parse("?watch=inProgress&q=a&date=2026-09")).toEqual({
      watch: "inProgress",
      query: "a",
      date: "2026-09",
    });
    expect(parse("?watch=watched&date=2026-09-27")).toEqual({
      watch: "watched",
      query: "",
      date: "2026-09-27",
    });
  });

  it("無い値と読めない値は既定として扱う", () => {
    expect(parse("")).toEqual({ watch: "all", query: "" });
    expect(parse("?watch=unwatched&q=%20%20&date=2026-13")).toEqual({
      watch: "all",
      query: "",
    });
    expect(parse("?watch=ALL&date=yesterday")).toEqual({ watch: "all", query: "" });
  });

  it("q は前後の空白を落とし、100 符号位置で切る", () => {
    const long = "😀".repeat(120);
    const { query } = parse(`?q=${encodeURIComponent(`  ${long}  `)}`);
    expect(Array.from(query)).toHaveLength(100);
    expect(query).toBe("😀".repeat(100));
  });
});

describe("parseHistoryDate", () => {
  it("暦にある日と月だけを読む", () => {
    expect(parseHistoryDate("2026-02-28")).toBe("2026-02-28");
    expect(parseHistoryDate("2028-02-29")).toBe("2028-02-29");
    expect(parseHistoryDate("2026-02-29")).toBeUndefined();
    expect(parseHistoryDate("2026-04-31")).toBeUndefined();
    expect(parseHistoryDate("2026-00")).toBeUndefined();
    expect(parseHistoryDate("2026-9")).toBeUndefined();
    expect(parseHistoryDate("2026-09-27T00:00")).toBeUndefined();
    expect(parseHistoryDate(null)).toBeUndefined();
  });
});

describe("serializeHistoryCriteria", () => {
  it("既定値を書かず、watch・q・date の順で書く", () => {
    expect(serializeHistoryCriteria({ watch: "all", query: "" }).toString()).toBe("");
    expect(
      serializeHistoryCriteria({
        date: "2026-09",
        query: " a ",
        watch: "inProgress",
      }).toString(),
    ).toBe("watch=inProgress&q=a&date=2026-09");
    expect(
      serializeHistoryCriteria({ watch: "all", query: "", date: "bad" }).toString(),
    ).toBe("");
  });

  it("読んだ条件を書くと同じ条件に戻る", () => {
    const criteria = parse("?date=2026-09-27&q=kyoto&watch=watched");
    expect(parseHistoryCriteria(serializeHistoryCriteria(criteria))).toEqual(criteria);
  });
});

describe("hasHistoryConditions", () => {
  it("状態・検索語・日のどれかがあれば条件がある", () => {
    expect(hasHistoryConditions({ watch: "all", query: "" })).toBe(false);
    expect(hasHistoryConditions({ watch: "watched", query: "" })).toBe(true);
    expect(hasHistoryConditions({ watch: "all", query: "a" })).toBe(true);
    expect(hasHistoryConditions({ watch: "all", query: "", date: "2026-09" })).toBe(true);
  });
});
