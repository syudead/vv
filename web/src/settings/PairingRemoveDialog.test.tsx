import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { UiText } from "../i18n";
import { ToastProvider } from "../ui/Toast";
import { PairingRemoveDialog, type PairingRemoveWords } from "./PairingRemoveDialog";

const words: PairingRemoveWords = {
  trigger: "Remove" as UiText,
  title: "Remove it?" as UiText,
  description: "It goes away." as UiText,
  action: "Yes, remove" as UiText,
  cancel: "Cancel" as UiText,
  done: "Removed" as UiText,
  failed: (reason) => `Couldn't remove: ${reason}` as UiText,
};

function renderDialog(remove: () => Promise<void>, onRemoved = vi.fn()) {
  render(
    <ToastProvider>
      <PairingRemoveDialog words={words} remove={remove} onRemoved={onRemoved} />
    </ToastProvider>,
  );
  return onRemoved;
}

describe("PairingRemoveDialog", () => {
  it("shows the failure inside the open dialog and lets the owner try again", async () => {
    const remove = vi
      .fn<() => Promise<void>>()
      .mockRejectedValueOnce(new Error("the other VVMDM is down"))
      .mockResolvedValueOnce(undefined);
    const user = userEvent.setup();
    const onRemoved = renderDialog(remove);

    await user.click(screen.getByRole("button", { name: "Remove" }));
    const dialog = screen.getByRole("alertdialog", { name: "Remove it?" });
    await user.click(within(dialog).getByRole("button", { name: "Yes, remove" }));

    expect(
      await within(dialog).findByText("Couldn't remove: Something went wrong."),
    ).toBeDefined();
    expect(onRemoved).not.toHaveBeenCalled();
    expect(
      within(dialog)
        .getByRole("button", { name: "Yes, remove" })
        .hasAttribute("disabled"),
    ).toBe(false);

    await user.click(within(dialog).getByRole("button", { name: "Yes, remove" }));
    expect(await screen.findByText("Removed")).toBeDefined();
    expect(onRemoved).toHaveBeenCalledTimes(1);
    expect(remove).toHaveBeenCalledTimes(2);
  });

  it("calls nothing when the owner cancels", async () => {
    const remove = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
    const user = userEvent.setup();
    const onRemoved = renderDialog(remove);

    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(remove).not.toHaveBeenCalled();
    expect(onRemoved).not.toHaveBeenCalled();
  });
});
