import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

function credit(id: number, name: string, role: WorkInfoValue["people"][number]["role"]) {
  return { role, person: { id, name, provisional: false }, provisional: false };
}

function renderInfo(info: WorkInfoValue) {
  return render(
    <MemoryRouter>
      <WorkInfo info={info} />
    </MemoryRouter>,
  );
}

/** Each row as "term: value". */
function rows() {
  return screen
    .getAllByRole("term")
    .map((term) => `${term.textContent!}: ${term.nextElementSibling!.textContent!}`);
}

describe("WorkInfo", () => {
  it("説明と出演者を除く項目を種類ごとのラベルで出す", () => {
    renderInfo({
      code: "AB-123",
      description: "説明",
      dates: [
        { kind: "release", date: "2024-03-01" },
        { kind: "recorded", date: "2023-08" },
        { kind: "published", date: "2024" },
      ],
      people: [credit(1, "出る人", "performer"), credit(2, "撮る人", "director")],
      organizations: [
        { role: "maker", organization: { id: 5, name: "Studio", provisional: false } },
        { role: "label", organization: { id: 6, name: "Line", provisional: false } },
      ],
      urls: ["https://works.example/w/1"],
      externalIds: [{ source: "works", value: "W-1" }],
    });
    expect(rows()).toEqual([
      "Code: AB-123",
      "Released: 2024-03-01",
      "Recorded: 2023-08",
      "Published: 2024",
      "Director: 撮る人",
      "Maker: Studio",
      "Label: Line",
      "Links: works.example/w/1",
      "IDs: works: W-1",
    ]);
    expect(screen.queryByText("出る人")).toBeNull();
    expect(screen.queryByText("説明")).toBeNull();
  });

  it("出演者と説明だけなら何も出さない", () => {
    const { container } = renderInfo({
      ...empty,
      description: "説明",
      people: [credit(1, "出る人", "performer")],
    });
    expect(container.textContent).toBe("");
  });

  it("4 人いる役割は 3 人と +1 more に畳み、開くと 4 人目へフォーカスする", async () => {
    renderInfo({
      ...empty,
      people: [1, 2, 3, 4].map((id) => credit(id, `撮る人${String(id)}`, "director")),
    });
    expect(rows()).toEqual(["Directors: 撮る人1, 撮る人2, 撮る人3 +1 more"]);
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Show all 4 directors" }));
    expect(document.activeElement).toBe(
      screen.getByRole("link", { name: "Open 撮る人4" }),
    );
    expect(screen.getByRole("button", { name: "Show fewer" })).toBeDefined();
  });

  it("人物は顔と名前で人物ページを開き、顔がなければ代わりの目印、団体はライブラリの絞り込みのまま", () => {
    renderInfo({
      ...empty,
      people: [
        { ...credit(4, "顔の人", "director"), imageRevision: 9 },
        credit(6, "顔なし", "director"),
      ],
      organizations: [
        { role: "maker", organization: { id: 5, name: "Studio", provisional: false } },
      ],
    });
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

  it("仮の人物は空の名前ではなく番号の顔と名前で人物ページを開く", () => {
    renderInfo({
      ...empty,
      people: [
        {
          role: "director",
          person: { id: 8, name: "", provisional: true },
          provisional: true,
          imageRevision: 2,
        },
      ],
    });
    const link = screen.getByRole("link", { name: "Open #8" });
    expect(link.getAttribute("href")).toBe("/people/8");
    expect(within(link).getByRole("img").getAttribute("alt")).toBe("#8");
    expect(link.textContent).toBe("#8");
  });
});
