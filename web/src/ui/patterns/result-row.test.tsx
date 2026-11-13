import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/ui/shadcn/tooltip";

import { ResultRow } from "./result-row";

function row(expanded: boolean, onExpandedChange: (expanded: boolean) => void) {
  return (
    <TooltipProvider>
      <ul>
        <ResultRow
          mark={<span>96%</span>}
          title="Harbour"
          expanded={expanded}
          onExpandedChange={onExpandedChange}
          toggleLabel={
            expanded
              ? { tooltip: "Hide frames", name: "Hide frames of Harbour" }
              : { tooltip: "Show frames", name: "Show frames of Harbour" }
          }
        >
          <p>frames</p>
        </ResultRow>
      </ul>
    </TooltipProvider>
  );
}

describe("ResultRow", () => {
  it("shows its body from the caller's expanded and names its button with toggleLabel", () => {
    const onExpandedChange = vi.fn();
    const { rerender } = render(row(false, onExpandedChange));

    expect(screen.queryByText("frames")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Show frames of Harbour" }));
    expect(onExpandedChange).toHaveBeenCalledWith(true);
    // The row stays collapsed until the caller passes the new state.
    expect(screen.queryByText("frames")).toBeNull();

    rerender(row(true, onExpandedChange));

    expect(screen.getByText("frames")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Hide frames of Harbour" })).toBeTruthy();
  });
});
