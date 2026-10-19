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
            people: [
              {
                role: "photographer",
                person: { id: 4, name: "撮る人", provisional: false },
                provisional: false,
              },
            ],
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
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });

  it("人物は顔と名前で人物ページを開き、顔がなければ代わりの目印、団体はライブラリの絞り込みのまま", () => {
    render(
      <MemoryRouter>
        <WorkInfo
          info={{
            ...empty,
            people: [
              {
                role: "performer",
                person: { id: 4, name: "顔の人", provisional: false },
                provisional: false,
                imageRevision: 9,
              },
              {
                role: "performer",
                person: { id: 6, name: "顔なし", provisional: false },
                provisional: false,
              },
            ],
            organizations: [
              {
                role: "maker",
                organization: { id: 5, name: "Studio", provisional: false },
              },
            ],
          }}
        />
      </MemoryRouter>,
    );
    const withFace = screen.getByRole("link", { name: "Open 顔の人" });
    expect(withFace.getAttribute("href")).toBe("/people/4");
    expect(within(withFace).getByRole("img").getAttribute("src")).toBe(
      "/api/people/4/image?v=9",
    );
    expect(withFace.textContent).toBe("顔の人");

    const withoutFace = screen.getByRole("link", { name: "Open 顔なし" });
    expect(withoutFace.getAttribute("href")).toBe("/people/6");
    expect(within(withoutFace).queryByRole("img")).toBeNull();
    expect(withoutFace.querySelector('[data-slot="person-face"]')).not.toBeNull();

    expect(
      screen.getByRole("link", { name: "Show videos with Studio" }).getAttribute("href"),
    ).toBe("/?organization=5");
    expect(screen.queryByRole("link", { name: /Show videos with 顔/ })).toBeNull();
  });

  it("仮の人物は空の名前ではなく Unnamed person の顔と名前で人物ページを開く", () => {
    render(
      <MemoryRouter>
        <WorkInfo
          info={{
            ...empty,
            people: [
              {
                role: "performer",
                person: { id: 8, name: "", provisional: true },
                provisional: true,
                imageRevision: 2,
              },
            ],
          }}
        />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link", { name: "Open Unnamed person" });
    expect(link.getAttribute("href")).toBe("/people/8");
    expect(within(link).getByRole("img").getAttribute("alt")).toBe("Unnamed person");
    expect(link.textContent).toBe("Unnamed person");
  });
});
