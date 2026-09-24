import { describe, expect, it } from "vitest";

import {
  clearConditions,
  directionToggleLabel,
  hasConditions,
  MAX_SEED,
  newSeed,
  parseListCriteria,
  parseSeed,
  SearchSession,
  serializeListCriteria,
  sortDirection,
  sortKindOf,
  sortKinds,
  videoSorts,
  withDirection,
} from "./listCriteria";

function parse(search: string, preferred?: Parameters<typeof parseListCriteria>[1]) {
  return parseListCriteria(new URLSearchParams(search), preferred);
}

describe("parseListCriteria（URL の解釈）", () => {
  it("何も無ければ既定の条件になる", () => {
    expect(parse("")).toEqual({
      criteria: { query: "", watch: "all", playable: false, sort: "addedDesc" },
      needsSeed: false,
      hasExplicitSort: false,
    });
  });

  it("今の形式の URL（?q=…&sort=addedDesc）は同じ意味になる", () => {
    expect(parse("?q=%E4%BA%AC%E9%83%BD&sort=addedDesc").criteria).toEqual({
      query: "京都",
      watch: "all",
      playable: false,
      sort: "addedDesc",
    });
    expect(parse("?sort=titleAsc").criteria.sort).toBe("titleAsc");
  });

  it("すべてのパラメータを読む", () => {
    expect(
      parse("?q=a+b&watch=inProgress&playable=1&sort=random&seed=123").criteria,
    ).toEqual({
      query: "a b",
      watch: "inProgress",
      playable: true,
      sort: "random",
      seed: 123,
    });
  });

  it("解釈できない値は既定として扱い、誤りにしない", () => {
    expect(parse("?watch=bogus&playable=true&sort=nope").criteria).toEqual({
      query: "",
      watch: "all",
      playable: false,
      sort: "addedDesc",
    });
    // 未知の sort は端末に保存した並び順に戻る。
    expect(parse("?sort=nope", "durationDesc").criteria.sort).toBe("durationDesc");
    expect(parse("?sort=nope", "durationDesc").hasExplicitSort).toBe(false);
  });

  it("sort が無ければ端末に保存した並び順を使う", () => {
    expect(parse("", "sizeAsc").criteria.sort).toBe("sizeAsc");
    expect(parse("?sort=titleDesc", "sizeAsc").criteria.sort).toBe("titleDesc");
  });

  it("13 の並び順をすべて読む", () => {
    for (const sort of videoSorts) {
      expect(parse(`?sort=${sort}&seed=5`).criteria.sort).toBe(sort);
    }
  });

  it("検索語は前後の空白を落とし、100 符号位置で切る", () => {
    expect(parse("?q=%20%20abc%20").criteria.query).toBe("abc");
    const long = "猫".repeat(120);
    expect(parse(`?q=${encodeURIComponent(long)}`).criteria.query).toBe("猫".repeat(100));
    // サロゲートペアを割らない
    const emoji = "😀".repeat(101);
    expect(
      Array.from(parse(`?q=${encodeURIComponent(emoji)}`).criteria.query),
    ).toHaveLength(100);
  });

  describe("seed の補い", () => {
    it("random で seed が無ければ補うよう知らせる", () => {
      const parsed = parse("?sort=random");
      expect(parsed.needsSeed).toBe(true);
      expect(parsed.criteria.seed).toBeUndefined();
    });

    it("壊れた seed や範囲外の seed も補う", () => {
      for (const seed of ["abc", "0", "-1", "1.5", String(MAX_SEED + 1), ""]) {
        expect(parse(`?sort=random&seed=${seed}`).needsSeed).toBe(true);
      }
      expect(parse(`?sort=random&seed=${String(MAX_SEED)}`).needsSeed).toBe(false);
      expect(parse("?sort=random&seed=1").criteria.seed).toBe(1);
    });

    it("端末に保存した並び順が random で URL に sort が無ければ、新しい seed で開く", () => {
      expect(parse("", "random").needsSeed).toBe(true);
    });

    it("random 以外では seed を無視する", () => {
      const parsed = parse("?sort=titleAsc&seed=5");
      expect(parsed.criteria.seed).toBeUndefined();
      expect(parsed.needsSeed).toBe(false);
    });
  });
});

describe("serializeListCriteria（URL への書き出し）", () => {
  it("既定の視聴状態と再生可否は書かない", () => {
    expect(
      serializeListCriteria({
        query: "",
        watch: "all",
        playable: false,
        sort: "addedDesc",
      }).toString(),
    ).toBe("sort=addedDesc");
  });

  it("決まった順で書き、読み戻すと同じ条件になる", () => {
    const criteria = {
      query: "京都 2024",
      watch: "unwatched" as const,
      playable: true,
      sort: "random" as const,
      seed: 99,
    };
    const params = serializeListCriteria(criteria);
    expect([...params.keys()]).toEqual(["q", "watch", "playable", "sort", "seed"]);
    expect(params.get("playable")).toBe("1");
    expect(parseListCriteria(params).criteria).toEqual(criteria);
  });

  it("random 以外では seed を書かない", () => {
    expect(
      serializeListCriteria({
        query: "",
        watch: "all",
        playable: false,
        sort: "titleAsc",
        seed: 3,
      }).has("seed"),
    ).toBe(false);
  });
});

describe("parseSeed と newSeed", () => {
  it("1 以上 MAX_SEED 以下の整数だけを受ける", () => {
    expect(parseSeed("42")).toBe(42);
    expect(parseSeed(null)).toBeUndefined();
    expect(parseSeed("1e3")).toBeUndefined();
  });

  it("範囲内の値を作り、前と同じ値を避ける", () => {
    expect(newSeed(undefined, () => 0)).toBe(1);
    expect(newSeed(undefined, () => 0.9999999999)).toBe(MAX_SEED);
    const values = [0, 0, 0.5];
    const next = newSeed(1, () => values.shift() ?? 0.5);
    expect(next).not.toBe(1);
  });
});

describe("並べ替えの種類と向き", () => {
  it("7 つの種類と、選んだときの向き", () => {
    expect(sortKinds.map((info) => [info.label, info.initial])).toEqual([
      ["追加日", "addedDesc"],
      ["更新日時", "modifiedDesc"],
      ["題名", "titleAsc"],
      ["長さ", "durationDesc"],
      ["ファイルサイズ", "sizeDesc"],
      ["最近再生した順", "playedDesc"],
      ["ランダム", "random"],
    ]);
  });

  it("向きを切り替えても種類は変わらない", () => {
    expect(withDirection("addedDesc", "asc")).toBe("addedAsc");
    expect(withDirection("titleAsc", "desc")).toBe("titleDesc");
    expect(withDirection("random", "asc")).toBe("random");
    expect(sortDirection("playedAsc")).toBe("asc");
    expect(sortDirection("random")).toBeUndefined();
    expect(sortKindOf("sizeAsc").kind).toBe("size");
  });

  it("向きの読み上げ名は今の向きと押したときの向きを含む", () => {
    expect(directionToggleLabel("addedDesc")).toBe("降順（新しい順）。押すと昇順");
    expect(directionToggleLabel("durationAsc")).toBe("昇順（短い順）。押すと降順");
    expect(directionToggleLabel("titleAsc")).toBe("昇順。押すと降順");
  });
});

describe("条件を解除", () => {
  it("検索語・視聴状態・再生可否を外し、並べ替えと seed は残す", () => {
    const criteria = {
      query: "京都",
      watch: "watched" as const,
      playable: true,
      sort: "random" as const,
      seed: 5,
    };
    expect(hasConditions(criteria)).toBe(true);
    const cleared = clearConditions(criteria);
    expect(cleared).toEqual({
      query: "",
      watch: "all",
      playable: false,
      sort: "random",
      seed: 5,
    });
    expect(hasConditions(cleared)).toBe(false);
  });
});

describe("SearchSession（検索語の入力の履歴）", () => {
  it("一続きの入力では最初の確定だけが履歴を増やす", () => {
    const session = new SearchSession();
    session.start();
    expect(session.commit()).toBe("push");
    expect(session.commit()).toBe("replace");
    expect(session.commit()).toBe("replace");
  });

  it("フォーカスが外れるか Esc で抜けると、次の続きでまた1つ増やす", () => {
    const session = new SearchSession();
    session.start();
    expect(session.commit()).toBe("push");
    session.end();
    session.start();
    expect(session.commit()).toBe("push");
    expect(session.commit()).toBe("replace");
  });
});
