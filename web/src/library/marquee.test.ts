import { describe, expect, it } from "vitest";

import { boxBetween, edgeSpeed, overlaps } from "./marquee";

describe("marquee", () => {
  it("どちら向きに引いても同じ四角になり、辺が接するだけでは重ならない", () => {
    const box = boxBetween({ x: 30, y: 40 }, { x: 10, y: 5 });
    expect(box).toEqual({ left: 10, top: 5, right: 30, bottom: 40 });
    expect(overlaps(box, { left: 29, top: 39, right: 50, bottom: 50 })).toBe(true);
    expect(overlaps(box, { left: 30, top: 0, right: 50, bottom: 50 })).toBe(false);
  });

  it("画面の上下の端に近いほど速く、端の外でも上限で、中ほどでは動かさない", () => {
    expect(edgeSpeed(400, 800)).toBe(0);
    expect(edgeSpeed(24, 800)).toBe(-12);
    expect(edgeSpeed(-30, 800)).toBe(-24);
    expect(edgeSpeed(800, 800)).toBe(24);
  });
});
