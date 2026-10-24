import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import VirtualLinks from "./VirtualLinks";

const urls = ["https://a.example/1", "https://b.example/2", "https://c.example/3"];

describe("VirtualLinks", () => {
  it("3 件までは全部をボタンで並べる", () => {
    render(<VirtualLinks urls={urls} />);
    const list = screen.getByRole("list", { name: "Links" });
    expect(
      within(list)
        .getAllByRole("link")
        .map((a) => a.getAttribute("href")),
    ).toEqual(urls);
    expect(screen.queryByRole("button", { name: /more$/ })).toBeNull();
  });

  it("4 件以上は 2 件を並べ、残りは +n more のメニューから新しいタブで開く", async () => {
    const many = [...urls, "https://d.example/4"];
    render(<VirtualLinks urls={many} />);
    const list = screen.getByRole("list", { name: "Links" });
    expect(
      within(list)
        .getAllByRole("link")
        .map((a) => a.getAttribute("href")),
    ).toEqual(many.slice(0, 2));
    await userEvent.setup().click(screen.getByRole("button", { name: "+2 more" }));
    const items = within(await screen.findByRole("menu")).getAllByRole("menuitem");
    expect(items.map((a) => [a.getAttribute("href"), a.getAttribute("target")])).toEqual([
      [many[2], "_blank"],
      [many[3], "_blank"],
    ]);
  });
});
