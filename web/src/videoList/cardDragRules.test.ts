import { describe, expect, it } from "vitest";

import {
  axisOf,
  liftedKeys,
  moveAfter,
  normalizeBetween,
  reorderDrop,
  signFor,
  zoneOf,
} from "./cardDragRules";

const order = ["a", "b", "c", "d", "e"];

describe("zoneOf", () => {
  it("splits a card into quarters: edges are between, the inner half is the middle", () => {
    expect(zoneOf(10, 0, 100, "quarters")).toBe("before");
    expect(zoneOf(50, 0, 100, "quarters")).toBe("middle");
    expect(zoneOf(80, 0, 100, "quarters")).toBe("after");
  });

  it("splits a card into halves when it is not a target to drop onto", () => {
    expect(zoneOf(49, 0, 100, "halves")).toBe("before");
    expect(zoneOf(51, 0, 100, "halves")).toBe("after");
  });
});

describe("signFor", () => {
  const lifted = new Set(["c"]);

  it("refuses a lifted card", () => {
    expect(
      signFor({ target: "c", lifted, reorder: true, onto: null, zone: "before" }),
    ).toEqual({ kind: "refused", key: "c" });
  });

  it("is between at an edge where reordering is allowed", () => {
    expect(
      signFor({
        target: "a",
        lifted,
        reorder: true,
        onto: { label: "Put in" },
        zone: "after",
      }),
    ).toEqual({ kind: "between", key: "a", side: "after" });
  });

  it("is onto in the middle, and onto at the edge where nothing is reordered", () => {
    const onto = { label: "Put in" };
    expect(signFor({ target: "a", lifted, reorder: true, onto, zone: "middle" })).toEqual(
      { kind: "onto", key: "a", label: "Put in" },
    );
    expect(
      signFor({ target: "a", lifted, reorder: false, onto, zone: "before" }),
    ).toEqual({ kind: "onto", key: "a", label: "Put in" });
  });

  it("follows the screen's refusal, and shows nothing on a card that is no target", () => {
    expect(
      signFor({ target: "a", lifted, reorder: false, onto: "refused", zone: "middle" }),
    ).toEqual({ kind: "refused", key: "a" });
    expect(
      signFor({ target: "a", lifted, reorder: false, onto: null, zone: "middle" }),
    ).toBeNull();
  });
});

describe("normalizeBetween", () => {
  it("names the place after a card as the place before the next one", () => {
    expect(normalizeBetween(order, "b", "after")).toEqual({ key: "c", side: "before" });
    expect(normalizeBetween(order, "e", "after")).toEqual({ key: "e", side: "after" });
    expect(normalizeBetween(order, "b", "before")).toEqual({ key: "b", side: "before" });
  });
});

describe("reorderDrop", () => {
  it("moves the third card to the top with no item before it", () => {
    expect(reorderDrop(order, new Set(["c"]), "a", "before")).toEqual({
      order: ["c", "a", "b", "d", "e"],
      moved: ["c"],
      after: null,
      position: 1,
    });
  });

  it("moves several lifted cards together in screen order after the card before the bar", () => {
    expect(reorderDrop(order, new Set(["e", "b"]), "d", "before")).toEqual({
      order: ["a", "c", "b", "e", "d"],
      moved: ["b", "e"],
      after: "c",
      position: 3,
    });
  });

  it("accepts a lifted card as the target of a renamed place", () => {
    expect(reorderDrop(order, new Set(["c", "d"]), "c", "before")).toBeNull();
    expect(reorderDrop(order, new Set(["a", "c"]), "c", "before")?.order).toEqual([
      "b",
      "a",
      "c",
      "d",
      "e",
    ]);
  });

  it("returns null when the order does not change", () => {
    expect(reorderDrop(order, new Set(["c"]), "d", "before")).toBeNull();
    expect(reorderDrop(order, new Set(["c"]), "x", "before")).toBeNull();
  });
});

describe("moveAfter", () => {
  it("puts the moved items after the given one, or first when it is null or missing", () => {
    const items = [1, 2, 3, 4];
    const id = (each: number) => each;
    expect(moveAfter(items, new Set([1, 3]), 4, id)).toEqual([2, 4, 1, 3]);
    expect(moveAfter(items, new Set([4]), null, id)).toEqual([4, 1, 2, 3]);
    expect(moveAfter(items, new Set([4]), 9, id)).toEqual([4, 1, 2, 3]);
  });
});

describe("liftedKeys", () => {
  it("lifts the whole selection in screen order when the pressed card is selected", () => {
    expect(liftedKeys(order, "d", true, ["d", "a"])).toEqual(["a", "d"]);
  });

  it("lifts the pressed card alone when it is not selected", () => {
    expect(liftedKeys(order, "b", false, ["d", "a"])).toEqual(["b"]);
  });
});

describe("axisOf", () => {
  const rect = (left: number, top: number) => ({ left, top, width: 10, height: 10 });

  it("is x when another card shares the row, else y", () => {
    const first = rect(0, 0);
    expect(axisOf(first, [first, rect(20, 0)])).toBe("x");
    expect(axisOf(first, [first, rect(0, 20)])).toBe("y");
  });
});
