import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/ui/shadcn/tooltip";

import { ResultRow, type ResultRowProps } from "./result-row";

function row(
  expanded: boolean,
  onExpandedChange: (expanded: boolean) => void,
  slots: Pick<ResultRowProps, "badge" | "meta" | "actions"> = {},
) {
  return (
    <TooltipProvider>
      <ul>
        <ResultRow
          mark={<span>96%</span>}
          title="Harbour"
          {...slots}
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

  it("puts the badge and meta with the title and the actions before the expand button", () => {
    render(
      row(false, vi.fn(), {
        badge: <span>Best candidate</span>,
        meta: <span>3 matching frames</span>,
        actions: <button type="button">Play</button>,
      }),
    );

    const title = screen.getByText("Harbour");
    expect(title.parentElement?.textContent).toBe("HarbourBest candidate");
    expect(screen.getByText("3 matching frames").parentElement?.dataset.slot).toBe(
      "result-row-meta",
    );
    const buttons = screen.getAllByRole("button");
    expect(buttons.map((b) => b.getAttribute("aria-label") ?? b.textContent)).toEqual([
      "Play",
      "Show frames of Harbour",
    ]);
  });
});
