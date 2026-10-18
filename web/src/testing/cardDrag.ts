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

export function dropOn(name: string | RegExp, from: number, to: number) {
  lift(name, from);
  moveTo(to);
  release(to);
}

/** Wait 50ms for dnd-kit's document click suppression before selecting the drop menu. */
export async function afterDrop() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 60));
  });
}

export async function openedMenu(): Promise<HTMLElement> {
  await afterDrop();
  return screen.findByRole("menu");
}
