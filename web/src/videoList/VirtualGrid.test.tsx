import { act, fireEvent, render } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import VirtualGrid, { columnsFor, type VirtualGridHandle } from "./VirtualGrid";

describe("columnsFor", () => {
  it.each([
    // [格子の幅, カードの幅, 狭い幅か, 列の数]
    [1280, 220, false, 5],
    [1280, 280, false, 4],
    [1280, 360, false, 3],
    [1280, 480, false, 2],
    [1920, 220, false, 8],
    [1920, 480, false, 3],
    [700, 480, false, 1],
    // カードの幅より格子が狭いときは格子の幅の 1 枚。
    [300, 480, false, 1],
    // sm 未満は倍率によらず 1 列。
    [375, 220, true, 1],
  ])("width %i, card %i, narrow %s → %i", (width, card, narrow, columns) => {
    expect(columnsFor(width, card, narrow)).toBe(columns);
  });
});

/** 格子の幅を 1280px、行の高さを 200px、窓の高さを 800px にする（jsdom にはレイアウトが無い）。 */
function useLayout() {
  let restore: (() => void)[] = [];
  beforeEach(() => {
    const rect = vi
      .spyOn(Element.prototype, "getBoundingClientRect")
      .mockImplementation(function (this: Element) {
        const index = (this as HTMLElement).dataset?.index;
        const top = index === undefined ? 0 : Number(index) * 210 - window.scrollY;
        return {
          width: 1280,
          height: 200,
          top,
          bottom: top + 200,
          left: 0,
          right: 1280,
          x: 0,
          y: top,
          toJSON: () => ({}),
        } as DOMRect;
      });
    const offset = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "offsetHeight");
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get: () => 200,
    });
    // 仮想化の部品は、スクロールの上限を文書の高さから決める。
    Object.defineProperty(document.documentElement, "scrollHeight", {
      configurable: true,
      get: () => 1_000_000,
    });
    const scrollY = Object.getOwnPropertyDescriptor(window, "scrollY");
    const height = window.innerHeight;
    window.innerHeight = 800;
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation((options) => {
      const top = typeof options === "object" ? (options.top ?? 0) : 0;
      Object.defineProperty(window, "scrollY", { configurable: true, value: top });
    });
    restore = [
      () => rect.mockRestore(),
      () => {
        if (offset !== undefined)
          Object.defineProperty(HTMLElement.prototype, "offsetHeight", offset);
      },
      () => {
        window.innerHeight = height;
      },
      () => scrollTo.mockRestore(),
      () => {
        if (scrollY !== undefined) Object.defineProperty(window, "scrollY", scrollY);
        else Reflect.deleteProperty(window, "scrollY");
      },
      () => Reflect.deleteProperty(document.documentElement, "scrollHeight"),
    ];
  });
  afterEach(() => {
    for (const undo of restore) undo();
  });
}

function grid(count: number, ref?: React.Ref<VirtualGridHandle>) {
  return (
    <VirtualGrid
      ref={ref}
      zoom={1}
      count={count}
      itemKey={(index) => `v:${String(index)}`}
      renderItem={(index) => (
        <a href={`/videos/${String(index)}`} data-testid="card">
          {index}
        </a>
      )}
    />
  );
}

describe("VirtualGrid", () => {
  it("draws every card when there is no layout (jsdom)", () => {
    const { getAllByTestId } = render(grid(50));
    expect(getAllByTestId("card")).toHaveLength(50);
  });

  describe("with layout", () => {
    useLayout();

    it("draws only the rows near the viewport", () => {
      const { getAllByTestId } = render(grid(1000));
      const drawn = getAllByTestId("card").length;
      // 1280px・倍率 1 は 4 列。窓 800px の 4 行と上下 2 行ずつの描き足しで、千枚よりずっと少ない。
      expect(drawn).toBeGreaterThan(0);
      expect(drawn).toBeLessThanOrEqual(4 * 10);
      expect(drawn % 4).toBe(0);
    });

    it("puts a partial last row in the same centred row as before", () => {
      const { container } = render(grid(6));
      const rows = container.querySelectorAll("[data-index]");
      expect(rows).toHaveLength(2);
      expect(rows[1]!.className).toContain("justify-center");
      expect(rows[1]!.children).toHaveLength(2);
    });

    it("scrolls the anchor item back to its offset", () => {
      const ref = createRef<VirtualGridHandle>();
      render(grid(1000, ref));
      act(() => ref.current!.restore({ key: "v:401", offset: 0 }));
      // 401 枚目は 4 列で 100 行目。行の間 10px を含む見積りの位置へ動く。
      const top = window.scrollY;
      expect(top).toBeGreaterThan(0);
      expect(window.scrollTo).toHaveBeenLastCalledWith({
        top: expect.any(Number) as number,
        behavior: "auto",
      });
    });

    it("restores the snapshot anchor when first drawn", () => {
      render(
        <VirtualGrid
          zoom={1}
          count={1000}
          itemKey={(index) => `v:${String(index)}`}
          renderItem={(index) => <span>{index}</span>}
          initialAnchor={{ key: "v:800", offset: 0 }}
        />,
      );
      expect(window.scrollY).toBeGreaterThan(0);
    });

    it("keeps the focused card's row drawn after scrolling away", () => {
      const { getByText, queryByText } = render(grid(1000));
      const first = getByText("0");
      act(() => first.focus());
      fireEvent.focus(first);
      act(() => {
        Object.defineProperty(window, "scrollY", { configurable: true, value: 50_000 });
        window.dispatchEvent(new Event("scroll"));
      });
      expect(queryByText("0")).not.toBeNull();
    });
  });
});
