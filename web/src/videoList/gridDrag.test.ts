import { describe, expect, it, vi } from "vitest";

import { createGridCards, createKeyedStore } from "./gridDrag";

describe("createKeyedStore", () => {
  it("keeps a key's selection until its own value changes", () => {
    const store = createKeyedStore(
      { active: "a" },
      (key, value: { active: string }) => ({ active: key === value.active }),
      (previous, next) => previous.active === next.active,
    );
    const listener = vi.fn();
    store.subscribe(listener);
    const a = store.select("a");
    const b = store.select("b");

    store.set({ active: "c" });
    expect(listener).toHaveBeenCalledTimes(1);
    expect(store.select("b")).toBe(b);
    expect(store.select("a")).not.toBe(a);
    expect(store.select("a")).toEqual({ active: false });
  });
});

describe("createGridCards", () => {
  it("measures each card once until a card registers or the rects are invalidated", () => {
    const cards = createGridCards();
    const node = document.createElement("div");
    const measure = vi
      .spyOn(node, "getBoundingClientRect")
      .mockReturnValue(new DOMRect(10, 20, 30, 40));
    cards.register("a", node);

    expect(cards.rects().get("a")).toEqual({
      top: 20,
      left: 10,
      width: 30,
      height: 40,
      bottom: 60,
      right: 40,
    });
    cards.rects();
    expect(measure).toHaveBeenCalledTimes(1);

    cards.invalidate();
    cards.rects();
    cards.register("b", document.createElement("div"));
    cards.rects();
    expect(measure).toHaveBeenCalledTimes(3);
  });
});
