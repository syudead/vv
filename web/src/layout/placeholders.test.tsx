import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import AppShell from "./AppShell";
import { navItems } from "./navigation";

function shell() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <AppShell>本文</AppShell>
    </MemoryRouter>,
  );
}

describe("ライブラリの骨格", () => {
  it("実装済みのライブラリ入口だけを表示する", () => {
    expect(navItems.map((item) => item.id)).toEqual([
      "all-videos",
      "recent",
      "in-progress",
    ]);

    const { container } = shell();
    expect(screen.getAllByRole("link")).toHaveLength(1);
    expect(container.querySelector('[data-nav-id="all-videos"]')?.tagName).toBe("A");
    expect(container.querySelector('[data-nav-id="recent"]')?.tagName).toBe("SPAN");
    expect(container.querySelector('[data-nav-id="in-progress"]')?.tagName).toBe("SPAN");
  });

  it("左上にロゴやページ見出しを表示しない", () => {
    shell();
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
    expect(screen.queryByText("vv")).toBeNull();
  });

  it("サイドバーの開閉状態を端末に保存する", () => {
    localStorage.clear();
    shell();
    fireEvent.click(screen.getByRole("button", { name: "サイドバーを折りたたむ" }));
    expect(localStorage.getItem("vv.sidebar.collapsed.v1")).toBe("true");
    expect(screen.getByRole("button", { name: "サイドバーを展開" })).toBeDefined();
  });
});
