import { describe, expect, it } from "vitest";

import { computeVisibleTagCount } from "./tagRowOverflow";

describe("computeVisibleTagCount", () => {
  it("すべて収まれば全部の個数を返す（+N は要らない）", () => {
    expect(computeVisibleTagCount([20, 30, 25], 24, 4, 200)).toBe(3);
  });

  it("収まらない分は +N の余白を残して切る", () => {
    // 20+4+30+4+25 = 83（全部収まる場合）。90 では収まらないので、
    // 最後の1個は +N（幅24・gap4）の分を差し引いた上で判定する。
    expect(computeVisibleTagCount([20, 30, 25], 24, 4, 80)).toBe(1);
  });

  it("幅が測れていない（0）ときはすべて表示する", () => {
    expect(computeVisibleTagCount([20, 30, 25], 24, 4, 0)).toBe(3);
  });

  it("1つも収まらないときは 0", () => {
    expect(computeVisibleTagCount([100], 24, 4, 10)).toBe(0);
  });
});
