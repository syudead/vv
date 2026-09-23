import { describe, expect, it } from "vitest";

import {
  breadcrumbsFor,
  folderUrl,
  parseFolderPathname,
  rootDisplayName,
} from "./folderPath";

describe("folderUrl と parseFolderPathname", () => {
  it.each(["A", "A/B/C", "100% #1 日本語", "a b/c?d", "%25 literal", "絵文字 🎬/x&y=z"])(
    "段 %s が URL の往復で同じフォルダに戻る",
    (path) => {
      const url = folderUrl({ rootId: 3, path });
      expect(url.startsWith("/folders/3/")).toBe(true);
      // URL として解釈しても、パス部分がそのまま残る（# や ? が区切りにならない）。
      const parsed = new URL(url, "http://localhost");
      expect(parsed.search).toBe("");
      expect(parsed.hash).toBe("");
      expect(parseFolderPathname(parsed.pathname)).toEqual({
        kind: "folder",
        folder: { rootId: 3, path },
      });
    },
  );

  it("登録フォルダ自身と最上位を区別する", () => {
    expect(folderUrl()).toBe("/folders");
    expect(folderUrl({ rootId: 7, path: "" })).toBe("/folders/7");
    expect(parseFolderPathname("/folders")).toEqual({ kind: "root" });
    expect(parseFolderPathname("/folders/")).toEqual({ kind: "root" });
    expect(parseFolderPathname("/folders/7")).toEqual({
      kind: "folder",
      folder: { rootId: 7, path: "" },
    });
  });

  it.each([
    "/folders/abc",
    "/folders/0",
    "/folders/3/%E0%A4%A",
    "/folders/3/..",
    "/folders/3/a//b",
    "/folders/3/a%2Fb",
    "/elsewhere",
  ])("解釈できない %s を invalid にする", (pathname) => {
    expect(parseFolderPathname(pathname)).toEqual({ kind: "invalid" });
  });
});

describe("rootDisplayName", () => {
  it("絶対パスの最後の段を名前にする", () => {
    expect(rootDisplayName("/a/movies")).toBe("movies");
    expect(rootDisplayName("/a/movies/")).toBe("movies");
    expect(rootDisplayName("D:\\media\\anime")).toBe("anime");
  });

  it("Unix のパスでは \\ を名前の一部として残す", () => {
    expect(rootDisplayName("/media/a\\b")).toBe("a\\b");
  });

  it("最後の段が無いパスはそのまま名前にする", () => {
    expect(rootDisplayName("/")).toBe("/");
    expect(rootDisplayName("D:\\")).toBe("D:\\");
  });
});

describe("breadcrumbsFor", () => {
  it("最上位・登録フォルダ・途中の段へのリンクを作り、現在地はリンクにしない", () => {
    expect(breadcrumbsFor({ rootId: 3, path: "A/B" }, "movies")).toEqual([
      { label: "フォルダ", to: "/folders" },
      { label: "movies", to: "/folders/3" },
      { label: "A", to: "/folders/3/A" },
      { label: "B" },
    ]);
  });

  it("登録フォルダの名前が分からない間はその段を空ける", () => {
    expect(breadcrumbsFor({ rootId: 3, path: "A" }, undefined)).toEqual([
      { label: "フォルダ", to: "/folders" },
      undefined,
      { label: "A" },
    ]);
  });
});
