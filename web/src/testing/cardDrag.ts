import { fireEvent, screen } from "@testing-library/react";
import { vi } from "vitest";

// ドラッグの層（videoList/cardDrag.tsx）のページテストの道具である。jsdom は配置を持たないので、
// 格子のカード（ドラッグの包み）を横一列に 200px ずつ置き、ポインタの出来事で持ち上げて落とす。

const cardWidth = 190;
const cardHeight = 150;
const step = 200;

/** layoutCards は格子のカード（`[data-drag-card]`）を左から 200px ずつ、同じ高さに置く。 */
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

/** pointOn は左から `index` 番目（0 始まり）のカードの、横の位置 `ratio` の点である。 */
function pointOn(index: number, ratio: number) {
  return { clientX: index * step + cardWidth * ratio, clientY: cardHeight / 2 };
}

const mouse = { pointerType: "mouse", isPrimary: true, button: 0, pointerId: 1 };

/** lift は名前 `name` のリンク（左から `index` 番目のカード）を押し、持ち上がるまで動かす。 */
export function lift(name: string | RegExp, index: number) {
  const link = screen.getByRole("link", { name });
  fireEvent.pointerDown(link, { ...mouse, ...pointOn(index, 0.5) });
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, 0.4) });
}

/** moveTo はポインタをカード `index` の位置 `ratio` へ動かす。 */
export function moveTo(index: number, ratio = 0.5) {
  fireEvent.pointerMove(document, { ...mouse, ...pointOn(index, ratio) });
}

/** release はカード `index` の位置 `ratio` で離す。 */
export function release(index: number, ratio = 0.5) {
  fireEvent.pointerUp(document, { ...mouse, ...pointOn(index, ratio) });
}

/** dropOn は名前 `name` のカード（左から `from` 番目）を、左から `to` 番目のカードの真ん中に落とす。 */
export function dropOn(name: string | RegExp, from: number, to: number) {
  lift(name, from);
  moveTo(to);
  release(to);
}
