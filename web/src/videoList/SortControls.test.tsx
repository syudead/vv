import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AudienceProvider } from "../auth/audience";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { CompactSortControls, SortMenu, type SortControlProps } from "./SortControls";

describe.each(["menu", "compact"] as const)("%s saved order", (layout) => {
  function Controls(props: SortControlProps) {
    return (
      <AudienceProvider audience="owner">
        <TooltipProvider>
          {layout === "menu" ? (
            <SortMenu {...props} />
          ) : (
            <CompactSortControls {...props} name="test-sort" />
          )}
        </TooltipProvider>
      </AudienceProvider>
    );
  }

  async function choose(user: ReturnType<typeof userEvent.setup>, name: string) {
    if (layout === "menu")
      await user.click(screen.getByRole("button", { name: /^Sort/ }));
    await user.click(
      screen.getByRole(layout === "menu" ? "menuitemradio" : "radio", { name }),
    );
  }

  it("保存順から同じ種類の一時ソートも選べ、保存順へ戻すと方向操作を隠す", async () => {
    const user = userEvent.setup();
    const props = {
      sort: "addedDesc" as const,
      onSortChange: vi.fn(),
      onShuffle: vi.fn(),
      nativeOrder: { active: true, onSelect: vi.fn() },
    };
    const { rerender } = render(<Controls {...props} />);
    expect(screen.queryByRole("button", { name: /oldest|newest|Shuffle/i })).toBeNull();
    await choose(user, "Date added");
    expect(props.onSortChange).toHaveBeenCalledExactlyOnceWith("addedDesc");
    expect(props.nativeOrder.onSelect).not.toHaveBeenCalled();

    rerender(
      <Controls {...props} nativeOrder={{ ...props.nativeOrder, active: false }} />,
    );
    await choose(user, "Saved order");
    expect(props.nativeOrder.onSelect).toHaveBeenCalledOnce();
    expect(props.onSortChange).toHaveBeenCalledOnce();
    expect(props.onShuffle).not.toHaveBeenCalled();
    rerender(<Controls {...props} />);
    expect(screen.queryByRole("group", { name: "Sort direction" })).toBeNull();
    expect(screen.queryByRole("button", { name: /oldest|newest|Shuffle/i })).toBeNull();
  });
});
