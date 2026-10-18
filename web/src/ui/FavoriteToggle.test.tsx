import { fireEvent, render, screen } from "@testing-library/react";
import { Tooltip } from "radix-ui";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import FavoriteToggle from "./FavoriteToggle";

describe("FavoriteToggle page", () => {
  it("shows the pressed state under its tooltip and toggles once without bubbling", () => {
    const onToggle = vi.fn(() => Promise.resolve());
    const parentClick = vi.fn();
    render(
      <Tooltip.Provider>
        <div onClick={parentClick}>
          <FavoriteToggle
            favorite
            label={t.designSystem.overlay.favoriteOn}
            onToggle={onToggle}
            variant="page"
          />
        </div>
      </Tooltip.Provider>,
    );

    const button = screen.getByRole("button", {
      name: t.designSystem.overlay.favoriteOn,
    });
    // Tooltip's closed data-state must not replace the toggle's on state.
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.getAttribute("data-state")).toBe("on");
    // Use aria-pressed to preserve favorite color while tooltip state replaces data-state.
    expect(button.className).toContain("aria-pressed:text-favorite");
    expect(button.className).not.toContain("aria-pressed:text-primary");

    fireEvent.click(button);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(parentClick).not.toHaveBeenCalled();
  });
});
