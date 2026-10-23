import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import FavoriteToggle from "./FavoriteToggle";

describe("FavoriteToggle page", () => {
  it("names itself with its visible word, shows the pressed state and toggles once without bubbling", () => {
    const onToggle = vi.fn(() => Promise.resolve());
    const parentClick = vi.fn();
    render(
      <div onClick={parentClick}>
        <FavoriteToggle
          favorite
          label={t.designSystem.overlay.favoriteOn}
          onToggle={onToggle}
          variant="page"
        />
      </div>,
    );

    const button = screen.getByRole("button", {
      name: t.designSystem.overlay.favoriteOn,
    });
    expect(button.textContent).toBe(t.designSystem.overlay.favoriteOn);
    expect(button.getAttribute("aria-pressed")).toBe("true");
    // The word takes the heart's colour, not the toggle's primary.
    expect(button.className).toContain("aria-pressed:text-favorite");
    expect(button.className).not.toContain("aria-pressed:text-primary");

    fireEvent.click(button);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(parentClick).not.toHaveBeenCalled();
  });
});
