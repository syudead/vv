import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import type { WorkInfo as WorkInfoValue } from "../api/workInfo";
import WorkInfo from "./WorkInfo";

const empty: WorkInfoValue = {
  dates: [],
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
};

describe("WorkInfo", () => {
  it("撮影日と撮影者だけなら、その 2 項目だけを出し、説明の Show more を出さない", () => {
    render(
      <MemoryRouter>
        <WorkInfo
          info={{
            ...empty,
            dates: [{ kind: "recorded", date: "2023-08" }],
            people: [{ role: "photographer", person: { id: 4, name: "撮る人" } }],
          }}
        />
      </MemoryRouter>,
    );
    const row = screen.getByRole("list", { name: "Work details" });
    expect(
      within(row)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Recorded2023-08", "Photographer撮る人"]);
    expect(
      within(row)
        .getByRole("link", { name: "Show videos with 撮る人" })
        .getAttribute("href"),
    ).toBe("/library?person=4");
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });
});
