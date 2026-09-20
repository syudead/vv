import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { ToastProvider } from "../ui/Toast";
import Sidebar from "./Sidebar";

describe("Sidebar", () => {
  it("閉じたドロワーをフォーカス順と支援技術から外す", () => {
    render(
      <MemoryRouter>
        <ToastProvider>
          <Sidebar mode="drawer" open={false} onClose={vi.fn()} />
        </ToastProvider>
      </MemoryRouter>,
    );

    const sidebar = document.querySelector("aside");
    expect(sidebar?.hasAttribute("inert")).toBe(true);
    expect(sidebar?.getAttribute("aria-hidden")).toBe("true");
  });

  it("設定をサイドバー末尾の実リンクとして表示してdrawerを閉じる", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();

    render(
      <MemoryRouter>
        <ToastProvider>
          <Sidebar mode="drawer" open onClose={onClose} />
        </ToastProvider>
      </MemoryRouter>,
    );

    const sidebar = screen.getByRole("complementary", {
      name: "メインナビゲーション",
    });
    const settings = within(sidebar).getByRole("link", { name: "設定" });

    await user.click(settings);

    expect(settings.getAttribute("href")).toBe("/settings");
    expect(onClose).toHaveBeenCalledOnce();
    expect(settings.getAttribute("aria-current")).toBe("page");
  });
});
