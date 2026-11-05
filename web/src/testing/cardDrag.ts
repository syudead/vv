import { act, fireEvent, screen } from "@testing-library/react";
import { vi } from "vitest";

// Supply 200px card positions because jsdom has no layout.

const cardWidth = 190;
const cardHeight = 150;
const step = 200;

export function layoutCards() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const card = this.closest("[data-drag-card]");
    const index =
      card === null ? 0 : Array.from(card.parentElement?.children ?? []).indexOf(card);
    const left = index * step;
    return {
      x: left,
      y: 0,
      top: 0,
      left,
      right: left + cardWidth,
      bottom: cardHeight,
      width: card === null ? 0 : cardWidth,
      height: card === null ? 0 : cardHeight,
      toJSON: () => ({}),
    } as DOMRect;
  });
}

function pointOn(index: number, ratio: number) {
  return { clientX: index * step + cardWidth * ratio, clientY: cardHeight / 2 };
}

export const mouse = { pointerType: "mouse", isPrimary: true, button: 0, pointerId: 1 };

export function lift(name: string | RegExp, index: number) {
  const link = screen.getByRole("link", { name });
  fireEvent.pointerDown(link, { ...mouse, ...pointOn(index, 0.5) });
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, 0.4) });
}

export function moveTo(index: number, ratio = 0.5) {
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, ratio) });
}

export function release(index: number, ratio = 0.5) {
  fireEvent.pointerUp(document, { ...mouse, ...pointOn(index, ratio) });
}

// dnd-kit stops every document click for this long after a drop.
const dropClickGuardMs = 50;

/** Drop on fake timers and pass dnd-kit's post-drop click guard, so the next click reaches the page. */
export async function releaseAndSettle(drop: () => void) {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  try {
    drop();
    await act(() => vi.advanceTimersByTimeAsync(dropClickGuardMs));
  } finally {
    vi.useRealTimers();
  }
}

/** Drag a card onto another and pass the post-drop click guard. */
export async function dropOn(name: string | RegExp, from: number, to: number) {
  lift(name, from);
  moveTo(to);
  await releaseAndSettle(() => release(to));
}

/** Find the drop menu; settle the drop with releaseAndSettle or dropOn first. */
export function openedMenu(): Promise<HTMLElement> {
  return screen.findByRole("menu");
}
