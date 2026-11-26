import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { type Tally, TallyStrip } from "./TallyStrip";

function tally(overrides: Partial<Tally>): Tally {
  return {
    id: "tentative",
    count: 3,
    label: "Tentative",
    pressed: false,
    onPressedChange: vi.fn(),
    ...overrides,
  };
}

describe("TallyStrip", () => {
  it("reports each tally's new state and keeps an empty one unpressable", async () => {
    const tentative = tally({});
    const unused = tally({ id: "unused", label: "Unused", pressed: true });
    const empty = tally({ id: "none", label: "No kind", count: 0, empty: true });
    render(
      <TallyStrip
        label="Left to tidy"
        clearLabel="Clear"
        tallies={[tentative, unused, empty]}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /Tentative/ }));
    await userEvent.click(screen.getByRole("button", { name: /Unused/ }));
    await userEvent.click(screen.getByRole("button", { name: /No kind/ }));

    expect(tentative.onPressedChange).toHaveBeenCalledWith(true);
    expect(unused.onPressedChange).toHaveBeenCalledWith(false);
    expect(empty.onPressedChange).not.toHaveBeenCalled();
  });
});
