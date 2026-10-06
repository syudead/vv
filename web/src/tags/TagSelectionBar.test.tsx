import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import TagSelectionBar from "./TagSelectionBar";

afterEach(cleanup);

function renderBar(busy: boolean, onClear = vi.fn()) {
  render(
    <TooltipProvider>
      <TagSelectionBar
        count={2}
        hasTentative
        hasConfirmed
        overLimit={false}
        busy={busy}
        confirming={busy}
        onConfirm={vi.fn()}
        onReject={vi.fn()}
        onDelete={vi.fn()}
        onMerge={vi.fn()}
        onClear={onClear}
      />
    </TooltipProvider>,
  );
  return onClear;
}

describe("TagSelectionBar", () => {
  it("まとめての操作の送信中は選択の解除を disabled にする", () => {
    renderBar(true);
    const clear = screen.getByRole("button", { name: t.tags.selection.clear });
    expect(clear).toHaveProperty("disabled", true);
  });

  it("送信中でなければ選択の解除を押せる", async () => {
    const onClear = renderBar(false);
    const clear = screen.getByRole("button", { name: t.tags.selection.clear });
    expect(clear).toHaveProperty("disabled", false);
    await userEvent.setup().click(clear);
    expect(onClear).toHaveBeenCalledTimes(1);
  });
});
