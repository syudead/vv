import { act, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { CardGrid, CardGridItem } from "./card-grid";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("limits a wide card to existing tracks when resizing or changing zoom", () => {
  let tracks = "280px 280px 280px";
  let measure = () => {};
  const disconnect = vi.fn();
  vi.spyOn(window, "getComputedStyle").mockImplementation(
    () => ({ gridTemplateColumns: tracks }) as CSSStyleDeclaration,
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        measure = callback;
      }
      observe() {}
      disconnect = disconnect;
    },
  );
  const grid = (size: 1 | 3) => (
    <CardGrid size={size}>
      <CardGridItem span={2}>
        <p>Folder</p>
      </CardGridItem>
      <p>Video</p>
    </CardGrid>
  );
  const { rerender, unmount } = render(grid(1));
  const item = screen.getByText("Folder").parentElement;
  expect(item?.style.gridColumn).toBe("span 2");
  tracks = "320px";
  act(() => measure());
  expect(item?.style.gridColumn).toBe("span 1");
  tracks = "480px 480px";
  rerender(grid(3));
  expect(item?.style.gridColumn).toBe("span 2");
  unmount();
  expect(disconnect).toHaveBeenCalledTimes(2);
});
