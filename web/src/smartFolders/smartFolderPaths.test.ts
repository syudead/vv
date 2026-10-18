import { describe, expect, it } from "vitest";

import {
  parseSmartFolderId,
  smartFolderContextOf,
  smartFolderCrumbs,
  smartFolderPagePath,
  smartFolderParentPath,
  videoInSmartFolderPath,
} from "./smartFolderPaths";

describe("smartFolderPaths", () => {
  it("ページと動画の場所を作り、経路の id を読む", () => {
    expect(smartFolderPagePath(3)).toBe("/smart-folders/3");
    expect(videoInSmartFolderPath(10, 3)).toBe("/videos/10?smartFolder=3");
    expect(parseSmartFolderId("3")).toBe(3);
    expect(parseSmartFolderId("x")).toBe(null);
  });

  it("動画ページの ?smartFolder を正の整数のときだけ読む", () => {
    expect(smartFolderContextOf("?smartFolder=3")).toBe(3);
    expect(smartFolderContextOf("?playlist=5&item=2&smartFolder=7")).toBe(7);
    expect(smartFolderContextOf("")).toBeUndefined();
    expect(smartFolderContextOf("?smartFolder=0")).toBeUndefined();
    expect(smartFolderContextOf("?smartFolder=x")).toBeUndefined();
  });

  it("帯はホーム、祖先（そのページへのリンク）、現在地の順", () => {
    expect(smartFolderCrumbs([{ id: 1, name: "タグ: 猫" }], "未視聴")).toEqual([
      { label: "Home", to: "/" },
      { label: "タグ: 猫", to: "/smart-folders/1" },
      { label: "未視聴" },
    ]);
  });
});

describe("smartFolderParentPath", () => {
  it("消した後は親のページへ、親が無ければライブラリへ移る", () => {
    expect(
      smartFolderParentPath([
        { id: 1, name: "猫" },
        { id: 4, name: "未視聴" },
      ]),
    ).toBe("/smart-folders/4");
    expect(smartFolderParentPath([])).toBe("/");
  });
});
