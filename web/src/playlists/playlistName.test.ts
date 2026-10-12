import { describe, expect, it } from "vitest";

import { playlistName } from "./playlistName";

describe("playlistName", () => {
  it("前後の空白を取り除いた名前を返す", () => {
    expect(playlistName("  Morning  ")).toBe("Morning");
  });

  it("サーバーと同じ Unicode White_Space を前後から取り除く", () => {
    // U+0085 は White_Space だが trim() は残す。U+FEFF は trim() が取るが White_Space ではない。
    expect(playlistName(`\u0085${"a".repeat(100)}\u0085`)).toBe("a".repeat(100));
    expect(playlistName("　Morning ")).toBe("Morning");
    expect(playlistName("﻿Morning﻿")).toBe("﻿Morning﻿");
  });

  it("空白だけの名前は送らない", () => {
    expect(playlistName("")).toBeNull();
    expect(playlistName("   ")).toBeNull();
  });

  it("コードポイントで数え、100 までは送り 101 からは送らない", () => {
    // 絵文字は UTF-16 で 2 単位だが 1 文字に数える（research.md R-11）。
    expect(playlistName("😀".repeat(100))).toBe("😀".repeat(100));
    expect(playlistName("😀".repeat(101))).toBeNull();
    expect(playlistName(` ${"a".repeat(100)} `)).toBe("a".repeat(100));
    expect(playlistName("a".repeat(101))).toBeNull();
  });
});
