import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import Breadcrumbs from "./Breadcrumbs";

// Assert responsive classes because breadcrumb collapsing is controlled entirely by CSS.
const crumbs = [
  { label: "Library", to: "/" },
  { label: "A", to: "/groups/1" },
  { label: "B", to: "/groups/2?trail=1" },
  { label: "C" },
];

function hiddenWhenNarrow(): string[] {
  const band = screen.getByRole("navigation");
  return within(band)
    .getAllByRole("listitem")
    .filter((item) => item.className.includes("hidden md:inline-flex"))
    .map((item) => item.textContent ?? "");
}

describe("Breadcrumbs", () => {
  it("4 段からは md 未満で親の手前を畳み、「… › 親 › 現在地」にする", () => {
    render(
      <MemoryRouter>
        <Breadcrumbs crumbs={crumbs} />
      </MemoryRouter>,
    );
    expect(hiddenWhenNarrow()).toEqual(["Library", "A"]);
  });

  it("keepFirst では最初の段を残し、「Library › … › 親 › 現在地」にする", () => {
    render(
      <MemoryRouter>
        <Breadcrumbs crumbs={crumbs} keepFirst />
      </MemoryRouter>,
    );
    expect(hiddenWhenNarrow()).toEqual(["A"]);
  });
});
