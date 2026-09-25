import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import type { VideoFolder } from "../api/client";
import { TooltipProvider } from "../ui/Tooltip";
import VideoHeader, { folderCrumbs } from "./VideoHeader";

function renderHeader(folder: VideoFolder | undefined, onClose = vi.fn()) {
  render(
    <TooltipProvider>
      <MemoryRouter>
        <VideoHeader folder={folder} onClose={onClose} />
      </MemoryRouter>
    </TooltipProvider>,
  );
  return onClose;
}

describe("VideoHeader", () => {
  it("ロゴはホームへのリンクで、× は 1 つだけ置き、押すと閉じる", () => {
    const onClose = renderHeader(undefined);
    expect(screen.getByRole("link", { name: "ホーム" }).getAttribute("href")).toBe("/");
    const close = screen.getAllByRole("button", { name: "閉じる" });
    expect(close).toHaveLength(1);
    fireEvent.click(close[0] as HTMLElement);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("登録フォルダから置き場所のフォルダまでの段を、各フォルダ画面へのリンクで並べる", () => {
    renderHeader({ rootId: 3, path: "2025/京都 旅行", rootName: "ホームビデオ" });
    const nav = screen.getByRole("navigation", { name: "フォルダ" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((link) => [link.textContent, link.getAttribute("href")])).toEqual([
      ["ホームビデオ", "/folders/3"],
      ["2025", "/folders/3/2025"],
      ["京都 旅行", "/folders/3/2025/%E4%BA%AC%E9%83%BD%20%E6%97%85%E8%A1%8C"],
    ]);
    // 狭い幅では最後の段だけを残し、途中は「…」に畳む（出し分けは CSS）。
    expect(links[0]?.closest("li")?.className.split(" ")).toEqual(
      expect.arrayContaining(["hidden", "md:flex"]),
    );
    expect(links[2]?.closest("li")?.className).not.toContain("hidden");
    expect(within(nav).getByText("…")).toBeDefined();
  });

  it("登録フォルダの直下なら段は 1 つで、「…」は出さない", () => {
    renderHeader({ rootId: 3, path: "", rootName: "movies" });
    const nav = screen.getByRole("navigation", { name: "フォルダ" });
    expect(
      within(nav)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["movies"]);
    expect(within(nav).queryByText("…")).toBeNull();
  });

  it("置き場所や登録フォルダの名前が分からなければパンくずを出さない", () => {
    expect(folderCrumbs(undefined)).toEqual([]);
    expect(folderCrumbs({ rootId: 3, path: "a" })).toEqual([]);
    renderHeader({ rootId: 3, path: "a" });
    expect(screen.queryByRole("navigation")).toBeNull();
  });
});
