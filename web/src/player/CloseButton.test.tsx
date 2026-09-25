import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "../ui/Tooltip";
import CloseButton from "./CloseButton";
import TouchControls from "./TouchControls";

describe("CloseButton", () => {
  it("プレイヤーに重ねる × は、見えない間は押せず、フォーカスが来たら見せる", () => {
    const onClose = vi.fn();
    const view = render(
      <TooltipProvider>
        <CloseButton variant="overlay" onClose={onClose} visible={false} />
      </TooltipProvider>,
    );
    const wrapper = view.container.querySelector("[data-close-overlay]");
    const classes = wrapper?.className.split(" ") ?? [];
    expect(classes).toContain("opacity-0");
    expect(classes).toContain("pointer-events-none");
    expect(classes).toContain("focus-within:opacity-100");

    view.rerender(
      <TooltipProvider>
        <CloseButton variant="overlay" onClose={onClose} visible />
      </TooltipProvider>,
    );
    expect(wrapper?.className.split(" ")).not.toContain("pointer-events-none");
    fireEvent.click(screen.getByRole("button", { name: "閉じる" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("TouchControls", () => {
  it("見えない間は押せず、Tab でフォーカスが来たら見せる", () => {
    const view = render(<TouchControls playing visible={false} onToggle={vi.fn()} />);
    const classes =
      view.container.querySelector("[data-touch-controls]")?.className.split(" ") ?? [];
    expect(classes).toContain("opacity-0");
    expect(classes).toContain("pointer-events-none");
    expect(classes).toContain("focus-within:opacity-100");
    expect(screen.getByRole("button", { name: "一時停止" })).toBeDefined();
  });
});
