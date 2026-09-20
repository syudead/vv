import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import TopBarPortal from "./TopBarPortal";

describe("TopBarPortal", () => {
  it("ページ固有の操作をトップバーのスロットへ移す", () => {
    render(
      <>
        <header>
          <div id="topbar-library-tools" />
        </header>
        <main>
          <TopBarPortal>
            <label>
              検索
              <input type="search" />
            </label>
          </TopBarPortal>
        </main>
      </>,
    );

    const header = screen.getByRole("banner");
    expect(within(header).getByRole("searchbox")).toBeDefined();
    expect(within(screen.getByRole("main")).queryByRole("searchbox")).toBeNull();
  });
});
