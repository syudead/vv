import type { KeyboardEvent } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { tabIntoUndrawnRow } from "./useVirtualTagRows";

function row(index: number): HTMLElement {
  const element = document.createElement("div");
  element.dataset.index = String(index);
  for (const part of ["first", "last"]) {
    const button = document.createElement("button");
    button.textContent = `${String(index)} ${part}`;
    element.append(button);
  }
  return element;
}

/** A list of rowCount rows with only drawn rows in the DOM; reveal draws a row as the virtualizer would. */
function list(drawn: number[]) {
  const element = document.createElement("div");
  for (const index of drawn) element.append(row(index));
  document.body.append(element);
  const reveal = vi.fn((index: number) => {
    const next = [...element.children].find(
      (child) => Number((child as HTMLElement).dataset.index) > index,
    );
    element.insertBefore(row(index), next ?? null);
  });
  return { element, reveal };
}

function button(container: HTMLElement, name: string): HTMLElement {
  return [...container.querySelectorAll("button")].find((b) => b.textContent === name)!;
}

function tab(container: HTMLElement, from: string, shiftKey = false) {
  const event = {
    key: "Tab",
    shiftKey,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    defaultPrevented: false,
    currentTarget: container,
    target: button(container, from),
    preventDefault: vi.fn(),
  };
  return event as unknown as KeyboardEvent<HTMLElement> & {
    preventDefault: typeof event.preventDefault;
  };
}

afterEach(() => {
  document.body.replaceChildren();
});

describe("tabIntoUndrawnRow", () => {
  it("Tab は行の最後から描画されていない次の行を描いてその最初へ、Shift+Tab は前の行の最後へ進む", () => {
    const forward = list([0]);
    const event = tab(forward.element, "0 last");
    tabIntoUndrawnRow(event, 3, forward.reveal);
    expect(forward.reveal).toHaveBeenCalledWith(1);
    expect(event.preventDefault).toHaveBeenCalled();
    expect(document.activeElement).toBe(button(forward.element, "1 first"));

    const backward = list([2]);
    tabIntoUndrawnRow(tab(backward.element, "2 first", true), 3, backward.reveal);
    expect(backward.reveal).toHaveBeenCalledWith(1);
    expect(document.activeElement).toBe(button(backward.element, "1 last"));
  });

  it("一覧の最後の行や行の途中からの Tab はブラウザに任せる", () => {
    const { element, reveal } = list([0, 2]);
    const fromEnd = tab(element, "2 last");
    tabIntoUndrawnRow(fromEnd, 3, reveal);
    const fromMiddle = tab(element, "0 first");
    tabIntoUndrawnRow(fromMiddle, 3, reveal);

    expect(reveal).not.toHaveBeenCalled();
    expect(fromEnd.preventDefault).not.toHaveBeenCalled();
    expect(fromMiddle.preventDefault).not.toHaveBeenCalled();
  });
});
