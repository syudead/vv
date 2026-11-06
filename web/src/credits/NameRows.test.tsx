import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { UiText } from "../i18n";
import type { FieldProblem } from "../player/detailsEditor/fields";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import {
  type NameRow,
  nameRowsControl,
  NameRows,
  nameRowsOf,
  nameRowsToSend,
} from "./NameRows";

const loaded = [
  { name: "Aoi", language: "en" },
  { name: "あおい", language: "ja" },
  { name: "葵" },
];

function Harness({
  initial,
  problem = null,
}: {
  initial: NameRow[];
  problem?: FieldProblem | null;
}) {
  const [rows, setRows] = useState(initial);
  return (
    <TooltipProvider>
      <NameRows
        rows={rows}
        onChange={setRows}
        saving={false}
        problem={problem}
        onEnter={() => undefined}
        title={"Names" as UiText}
      />
    </TooltipProvider>
  );
}

function names(): string[] {
  return screen
    .getAllByRole<HTMLInputElement>("textbox", { name: "Name" })
    .map((input) => input.value);
}

function displayName(): string {
  const slot = screen.getByRole("group", { name: "Display name" });
  return within(slot).getByRole<HTMLInputElement>("textbox", { name: "Name" }).value;
}

// dnd-kit's keyboard sorting reads row geometry, which jsdom does not lay out.
function layoutRows() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
    this: Element,
  ) {
    const row = this.closest("[data-name-row]");
    const index =
      row === null
        ? 0
        : Array.from(document.querySelectorAll("[data-name-row]")).indexOf(row);
    const top = index * 48;
    return {
      x: 0,
      y: top,
      top,
      left: 0,
      right: 600,
      bottom: top + 48,
      width: 600,
      height: 48,
      toJSON: () => ({}),
    } as DOMRect;
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("NameRows", () => {
  it("Make display name moves the row under Display name and offers it on the former one", async () => {
    render(<Harness initial={nameRowsOf(loaded)} />);

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: 'Make "葵" the display name' }));

    expect(names()).toEqual(["葵", "Aoi", "あおい"]);
    expect(displayName()).toBe("葵");
    expect(
      screen.queryByRole("button", { name: 'Make "葵" the display name' }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: 'Make "Aoi" the display name' }),
    ).toBeDefined();
  });

  it("keyboard sorting moves the picked-up row to the slot it is dropped in", async () => {
    layoutRows();
    render(<Harness initial={nameRowsOf(loaded)} />);
    const user = userEvent.setup();

    screen.getByRole("button", { name: 'Reorder "Aoi"' }).focus();
    await user.keyboard("[Space]");
    await user.keyboard("[ArrowDown]");
    await user.keyboard("[Space]");

    expect(names()).toEqual(["あおい", "Aoi", "葵"]);
    expect(displayName()).toBe("あおい");
    // The row changed slots, so React mounted it anew; dnd-kit puts the focus back on its handle.
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: 'Reorder "Aoi"' }),
      ),
    );
  });

  it("a problem at names[1].name marks the second row's name input", () => {
    const rows = nameRowsOf(loaded);
    const control = nameRowsControl("names[1].name", nameRowsToSend(rows).keys);
    render(
      <Harness
        initial={rows}
        problem={{
          control: control!,
          text: "This name is already in the list." as UiText,
        }}
      />,
    );

    const inputs = screen.getAllByRole("textbox", { name: "Name" });
    expect(inputs.map((input) => input.getAttribute("aria-invalid"))).toEqual([
      null,
      "true",
      null,
    ]);
    expect(
      document.getElementById(inputs[1]!.getAttribute("aria-describedby")!)?.textContent,
    ).toBe("This name is already in the list.");
  });
});

describe("nameRowsToSend", () => {
  it("sends a blank row among others at its index, so its reason lands on that row", () => {
    const rows = nameRowsOf([{ name: "Aoi" }, { name: " " }]);

    expect(nameRowsToSend(rows)).toEqual({
      values: [{ name: "Aoi" }, { name: "" }],
      keys: rows.map((row) => row.key),
    });
  });
});
