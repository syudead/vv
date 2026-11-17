import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { RateRing } from "./RateRing";

describe("RateRing", () => {
  it("shows the rate as the nearest whole percent and takes no focus", async () => {
    const { container } = render(
      <>
        <RateRing value={92.6} tone="muted" />
        <a href="#next">next</a>
      </>,
    );

    expect(container.querySelector("[data-slot=rate-ring]")?.textContent).toBe("93%");
    await userEvent.tab();
    expect(document.activeElement).toBe(screen.getByRole("link", { name: "next" }));
  });
});
