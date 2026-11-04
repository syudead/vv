import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/ui/shadcn/tooltip";

import { SelectionBar } from "./selection-bar";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("SelectionBar", () => {
  it("publishes the bottom bar's height for notifications above it and removes it on unmount", () => {
    let resize: () => void = () => undefined;
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(callback: () => void) {
          resize = callback;
        }
        observe() {}
        disconnect() {}
      },
    );
    const height = vi
      .spyOn(HTMLElement.prototype, "offsetHeight", "get")
      .mockReturnValue(46);
    const root = document.documentElement;
    const bar = (
      <TooltipProvider>
        <SelectionBar count="2 selected" clearLabel="Clear" onClear={() => undefined}>
          <span>Edit</span>
        </SelectionBar>
      </TooltipProvider>
    );

    const { unmount } = render(bar);
    expect(root.style.getPropertyValue("--selection-bar-height")).toBe("46px");
    // The actions wrap onto a second row.
    height.mockReturnValue(118);
    resize();
    expect(root.style.getPropertyValue("--selection-bar-height")).toBe("118px");

    unmount();
    expect(root.style.getPropertyValue("--selection-bar-height")).toBe("");
  });
});
