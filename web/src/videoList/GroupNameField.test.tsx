import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "../ui/shadcn/tooltip";
import { GroupNameField } from "./GroupNameField";
import { VideoDropMenu } from "./VideoDropMenu";

// 新しいグループの名前の欄と、動画を動画に落としたメニュー
//

describe("GroupNameField", () => {
  it("仮の名前を選んだ状態でフォーカスを受け、変えた名前の Enter で送って閉じる", async () => {
    const onRename = vi.fn();
    const onClose = vi.fn();
    render(<GroupNameField provisional="Trip" onRename={onRename} onClose={onClose} />);
    const field = screen.getByRole("textbox", { name: "Group name" }) as HTMLInputElement;
    expect(document.activeElement).toBe(field);
    expect([field.selectionStart, field.selectionEnd]).toEqual([0, 4]);
    expect(field.getAttribute("aria-describedby")).not.toBeNull();

    await userEvent.setup().keyboard("Kyoto{Enter}");

    expect(onRename).toHaveBeenCalledWith("Kyoto");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("100 文字を超えると規則を出し、Enter では閉じない", async () => {
    const onClose = vi.fn();
    render(<GroupNameField provisional="Trip" onRename={vi.fn()} onClose={onClose} />);
    const user = userEvent.setup();
    await user.keyboard(`${"a".repeat(101)}{Enter}`);

    expect(screen.getByRole("alert").textContent).toBe(
      "Use a name of 1 to 100 characters.",
    );
    expect(onClose).not.toHaveBeenCalled();
  });

  it("Esc は何も送らずに閉じる", async () => {
    const onRename = vi.fn();
    const onClose = vi.fn();
    render(<GroupNameField provisional="Trip" onRename={onRename} onClose={onClose} />);
    await userEvent.setup().keyboard("Kyoto{Escape}");

    expect(onRename).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("VideoDropMenu", () => {
  it("20 枚を超えると「Bundle as versions」を押せなくして理由を添える", () => {
    render(
      <TooltipProvider>
        <div className="relative">
          <VideoDropMenu
            name="B"
            liftedCount={20}
            onGroup={vi.fn()}
            onBundle={vi.fn()}
            onClose={vi.fn()}
          />
        </div>
      </TooltipProvider>,
    );
    const bundle = screen.getByRole("menuitem", { name: "Bundle as versions" });
    expect(bundle.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Bundle up to 20 videos at a time")).toBeDefined();
  });
});
