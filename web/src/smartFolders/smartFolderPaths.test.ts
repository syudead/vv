import { describe, expect, it } from "vitest";

import {
  parseSmartFolderId,
  smartFolderCrumbs,
  smartFolderPagePath,
  videoInSmartFolderPath,
} from "./smartFolderPaths";

describe("smartFolderPaths", () => {
  it("ページと動画の場所を作り、経路の id を読む", () => {
    expect(smartFolderPagePath(3)).toBe("/smart-folders/3");
    expect(videoInSmartFolderPath(10, 3)).toBe("/videos/10?smartFolder=3");
    expect(parseSmartFolderId("3")).toBe(3);
    expect(parseSmartFolderId("x")).toBe(null);
  });

  it("帯はライブラリ、祖先（そのページへのリンク）、現在地の順", () => {
    expect(smartFolderCrumbs([{ id: 1, name: "タグ: 猫" }], "未視聴")).toEqual([
      { label: "Library", to: "/" },
      { label: "タグ: 猫", to: "/smart-folders/1" },
      { label: "未視聴" },
    ]);
  });
});
