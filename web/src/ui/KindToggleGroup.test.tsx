import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { kindColour } from "./KindMark";
import { KindToggleGroup } from "./KindToggleGroup";

const options = [
  { value: "all", label: "All" },
  { value: "9", label: "Place", kind: 9 },
];

describe("KindToggleGroup", () => {
  it("reports a pressed kind and ignores pressing the pressed choice again", async () => {
    const onValueChange = vi.fn();
    render(
      <KindToggleGroup
        label="Kind"
        options={options}
        value="all"
        onValueChange={onValueChange}
      />,
    );

    await userEvent.click(screen.getByRole("radio", { name: "Place" }));
    await userEvent.click(screen.getByRole("radio", { name: "All" }));

    expect(onValueChange.mock.calls).toEqual([["9"]]);
  });
});

describe("kindColour", () => {
  it("keeps a kind's colour from its id alone, wrapping after eight", () => {
    expect([1, 8, 9, 16, 17].map(kindColour)).toEqual([1, 8, 1, 8, 1]);
  });
});
