import { describe, expect, it } from "vitest";

import { contextLine, routeOf, turnContext } from "./screenContext";

describe("routeOf", () => {
  it("ルートの種類と ID を、id を持つ画面だけ ID 付きで返す", () => {
    expect(routeOf("/")).toEqual({ kind: "library" });
    expect(routeOf("/videos/12")).toEqual({ kind: "video", id: 12 });
    expect(routeOf("/people/3/faces")).toEqual({ kind: "person", id: 3 });
    expect(routeOf("/people")).toEqual({ kind: "people" });
    expect(routeOf("/smart-folders/8")).toEqual({ kind: "smartFolder", id: 8 });
    expect(routeOf("/settings/connections")).toEqual({ kind: "settings" });
    expect(routeOf("/history")).toEqual({ kind: "other" });
  });
});

describe("turnContext", () => {
  it("ライブラリでは URL の絞り込みと明示の並び順を filter と sort で送る", () => {
    const context = turnContext(
      { pathname: "/", search: "?tag=4&tag=2&fav=1&sort=titleAsc&watchState=watched" },
      {},
      [],
    );
    expect(context).toEqual({
      route: { kind: "library" },
      filter: { favorite: true, tags: [4, 2], watchState: "watched" },
      sort: "titleAsc",
    });
  });

  it("ライブラリ以外の画面では URL の条件を filter に入れない", () => {
    expect(turnContext({ pathname: "/folders", search: "?tag=4" }, {}, [])).toEqual({
      route: { kind: "folder" },
    });
  });
});

describe("contextLine", () => {
  it("選択、絞り込み、開いている物の名前、その他の順に指す物を言う", () => {
    const library = { pathname: "/", search: "?minDurationMs=60000" };
    expect(contextLine(library, { videoIds: [1], count: 1 })).toBe(
      "Talking about 1 selected video",
    );
    expect(contextLine(library, {})).toBe("Talking about the filtered library");
    expect(contextLine({ pathname: "/videos/2", search: "" }, { name: "夏の旅" })).toBe(
      "Talking about “夏の旅”",
    );
    expect(contextLine({ pathname: "/groups/5", search: "" }, { name: "京都" })).toBe(
      "Talking about 京都",
    );
    expect(contextLine({ pathname: "/", search: "" }, {})).toBe(
      "Talking about this screen",
    );
  });
});
