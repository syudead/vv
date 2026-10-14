import { describe, expect, it } from "vitest";

import {
  childGroupPath,
  folderDisplayPath,
  groupContextOf,
  groupItemPath,
  groupPagePath,
  loadingCrumbs,
  parseGroupId,
  parseTrail,
  trailCrumbs,
  videoInGroupPath,
} from "./groupPaths";

describe("parseGroupId", () => {
  it("正の整数だけを読む", () => {
    expect(parseGroupId("7")).toBe(7);
    expect(parseGroupId("07")).toBeNull();
    expect(parseGroupId("0")).toBeNull();
    expect(parseGroupId("x")).toBeNull();
    expect(parseGroupId(undefined)).toBeNull();
    expect(parseGroupId("99999999999999999999")).toBeNull();
  });
});

describe("parseTrail", () => {
  it("外側から順に読み、読めない段は捨てる", () => {
    expect(parseTrail("?trail=1,5")).toEqual([1, 5]);
    expect(parseTrail("?trail=1,x,,5")).toEqual([1, 5]);
    expect(parseTrail("?trail=")).toEqual([]);
    expect(parseTrail("")).toEqual([]);
  });

  it("100 段までにする", () => {
    const ids = Array.from({ length: 120 }, (_, index) => index + 1);
    expect(parseTrail(`?trail=${ids.join(",")}`)).toHaveLength(100);
  });
});

describe("groupPagePath", () => {
  it("道筋が空なら問い合わせを付けない", () => {
    expect(groupPagePath(7)).toBe("/groups/7");
    expect(groupPagePath(7, [1, 5])).toBe("/groups/7?trail=1,5");
  });
});

describe("childGroupPath と groupItemPath", () => {
  const trail = [
    { id: 1, name: "Season" },
    { id: 5, name: "Extras" },
  ];

  it("子グループは道筋を 1 段伸ばして開く", () => {
    expect(childGroupPath(9, trail, 7)).toBe("/groups/9?trail=1,5,7");
    expect(childGroupPath(9, [], 7)).toBe("/groups/9?trail=7");
    expect(groupItemPath({ kind: "group", group: { id: 9 } }, trail, 7)).toBe(
      "/groups/9?trail=1,5,7",
    );
  });

  it("動画は ?group を付けて開く", () => {
    expect(videoInGroupPath(42, 7)).toBe("/videos/42?group=7");
    expect(groupItemPath({ kind: "video", video: { id: 42 } }, trail, 7)).toBe(
      "/videos/42?group=7",
    );
  });
});

describe("groupContextOf", () => {
  it("動画ページの ?group を正の整数として読み、読めなければ undefined", () => {
    expect(groupContextOf("?group=7")).toBe(7);
    expect(groupContextOf("?playlist=5&item=2&group=3")).toBe(3);
    expect(groupContextOf("")).toBeUndefined();
    expect(groupContextOf("?group=0")).toBeUndefined();
    expect(groupContextOf("?group=x")).toBeUndefined();
  });
});

describe("trailCrumbs", () => {
  it("Library、道筋のグループ（その手前で切った道筋付き）、開いているグループの順", () => {
    expect(
      trailCrumbs(
        [
          { id: 1, name: "Season" },
          { id: 5, name: "Extras" },
        ],
        "Bonus",
      ),
    ).toEqual([
      { label: "Library", to: "/" },
      { label: "Season", to: "/groups/1" },
      { label: "Extras", to: "/groups/5?trail=1" },
      { label: "Bonus" },
    ]);
  });

  it("読み込み中は道筋の段と開いているグループの段を名前の分からない段にする", () => {
    expect(loadingCrumbs([1, 5])).toEqual([
      { label: "Library", to: "/" },
      undefined,
      undefined,
      undefined,
    ]);
  });
});

describe("folderDisplayPath", () => {
  it("登録フォルダの名前から始め、名前が分からない間は相対パスだけにする", () => {
    expect(folderDisplayPath({ rootId: 1, path: "anime/s1" }, "Media")).toBe(
      "Media/anime/s1",
    );
    expect(folderDisplayPath({ rootId: 1, path: "" }, "Media")).toBe("Media");
    expect(folderDisplayPath({ rootId: 1, path: "anime/s1" })).toBe("anime/s1");
  });
});
