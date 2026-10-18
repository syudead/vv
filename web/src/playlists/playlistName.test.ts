import { describe, expect, it } from "vitest";

import { playlistName } from "./playlistName";

describe("playlistName", () => {
  it("前後の空白を取り除いた名前を返す", () => {
    expect(playlistName("  Morning  ")).toBe("Morning");
  });

  it("サーバーと同じ Unicode White_Space を前後から取り除く", () => {
    // trim() preserves U+0085 but removes U+FEFF, opposite to Unicode White_Space.
    expect(playlistName(`\u0085${"a".repeat(100)}\u0085`)).toBe("a".repeat(100));
    expect(playlistName("　Morning ")).toBe("Morning");
    expect(playlistName("﻿Morning﻿")).toBe("﻿Morning﻿");
  });

  it("空白だけの名前は送らない", () => {
    expect(playlistName("")).toBeNull();
    expect(playlistName("   ")).toBeNull();
  });

  it("コードポイントで数え、100 までは送り 101 からは送らない", () => {
    expect(playlistName("😀".repeat(100))).toBe("😀".repeat(100));
    expect(playlistName("😀".repeat(101))).toBeNull();
    expect(playlistName(` ${"a".repeat(100)} `)).toBe("a".repeat(100));
    expect(playlistName("a".repeat(101))).toBeNull();
  });
});
