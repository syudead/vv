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

    fireEvent.click(button);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(parentClick).not.toHaveBeenCalled();
  });
});
