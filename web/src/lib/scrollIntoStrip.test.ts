import { scrollIntoStrip } from "./scrollIntoStrip";

function setBox(element: HTMLElement, left: number, width: number) {
  element.getBoundingClientRect = () =>
    ({ left, width, top: 0, height: 0, right: left + width, bottom: 0 }) as DOMRect;
}

function strip(scrollLeft: number) {
  const root = document.createElement("div");
  Object.defineProperty(root, "clientWidth", { value: 300 });
  root.scrollLeft = scrollLeft;
  setBox(root, 16, 300);
  return root;
}

describe("scrollIntoStrip", () => {
  it("centres an item that sits past the right edge", () => {
    const root = strip(0);
    const item = document.createElement("button");
    setBox(item, 16 + 400, 100);
    scrollIntoStrip(root, item);
    expect(root.scrollLeft).toBe(300);
  });

  it("measures from the current scroll position", () => {
    const root = strip(120);
    const item = document.createElement("button");
    setBox(item, 16 + 400 - 120, 100);
    scrollIntoStrip(root, item);
    expect(root.scrollLeft).toBe(300);
  });

  it("does not scroll before the left edge", () => {
    const root = strip(50);
    const item = document.createElement("button");
    setBox(item, 16 - 50, 80);
    scrollIntoStrip(root, item);
    expect(root.scrollLeft).toBe(0);
  });
});
