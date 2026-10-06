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
    // ツールチップの起点の data-state（closed）が、Toggle の押した状態の "on" を上書きしない。
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.getAttribute("data-state")).toBe("on");
    // ツールチップが開いて data-state が置き換わっても、ハートは aria-pressed で
    // text-favorite のまま（Toggle の既定の aria-pressed:text-primary は残らない）。
    expect(button.className).toContain("aria-pressed:text-favorite");
    expect(button.className).not.toContain("aria-pressed:text-primary");

    fireEvent.click(button);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(parentClick).not.toHaveBeenCalled();
  });
});
