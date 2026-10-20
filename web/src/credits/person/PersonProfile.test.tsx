import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Person } from "../../api/credits";
import { PersonDetails } from "./PersonProfile";

const bioPerson: Person = {
  id: 7,
  name: "Aoi",
  provisional: false,
  aliases: [],
  externalIds: [],
  profile: { bio: "A long biography." },
  favorite: false,
  videoCount: 1,
  createdAt: "2026-01-01T00:00:00Z",
};

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("PersonDetails", () => {
  it("offers Show more once a narrower column makes the bio overflow", () => {
    // The callbacks of observers that watch an element, as a column resize would call them.
    const watching = new Set<() => void>();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        private readonly callback: () => void;
        constructor(callback: () => void) {
          this.callback = callback;
        }
        observe() {
          watching.add(this.callback);
        }
        disconnect() {
          watching.delete(this.callback);
        }
      },
    );
    const height = vi
      .spyOn(HTMLElement.prototype, "scrollHeight", "get")
      .mockReturnValue(0);
    render(
      <MemoryRouter>
        <PersonDetails person={bioPerson} today={new Date(2026, 0, 1)} />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();

    height.mockReturnValue(200);
    act(() => watching.forEach((callback) => callback()));

    expect(screen.getByRole("button", { name: "Show more" })).toBeTruthy();
  });
});
