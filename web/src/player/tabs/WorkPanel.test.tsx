import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import type { WorkInfo } from "../../api/workInfo";
import WorkPanel from "./WorkPanel";

const info: WorkInfo = {
  code: "A-1",
  dates: [],
  people: [],
  organizations: [],
  urls: ["https://example.com/a-very-long-path"],
  externalIds: [],
};

function renderPanel(canFetch: boolean, onFetch = vi.fn()) {
  render(
    <MemoryRouter>
      <WorkPanel info={info} canFetch={canFetch} onFetch={onFetch} />
    </MemoryRouter>,
  );
  return onFetch;
}

describe("WorkPanel", () => {
  it("取得の入口は一覧より前の見出しの行に通常の大きさで置き、押すと取得を開く（フィードバック）", () => {
    const onFetch = renderPanel(true);
    const button = screen.getByRole("button", { name: "Fetch again" });
    expect(button.getAttribute("data-size")).toBe("default");
    expect(button.getAttribute("aria-describedby")).not.toBeNull();
    const firstTerm = screen.getAllByRole("term")[0]!;
    expect(
      button.compareDocumentPosition(firstTerm) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    fireEvent.click(button);
    expect(onFetch).toHaveBeenCalledTimes(1);
  });

  it("承認された取得元が無ければ入口を出さない", () => {
    renderPanel(false);
    expect(screen.queryByRole("button", { name: /Fetch/ })).toBeNull();
    expect(screen.getByText("A-1")).toBeDefined();
  });
});
