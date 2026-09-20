import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import PlaybackFilter, { type PlaybackFilterValue } from "./PlaybackFilter";

describe("PlaybackFilter", () => {
  it("再生状態を選ぶと通知してメニューを閉じる", () => {
    const changes: PlaybackFilterValue[] = [];
    const { container } = render(
      <PlaybackFilter value="all" onChange={(value) => changes.push(value)} />,
    );

    const summary = screen.getByText("絞り込み").closest("summary");
    if (summary === null) {
      throw new Error("絞り込みの入口がない");
    }
    fireEvent.click(summary);
    expect(container.querySelector("details")?.hasAttribute("open")).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "視聴済み" }));
    expect(changes).toEqual(["watched"]);
    expect(container.querySelector("details")?.hasAttribute("open")).toBe(false);
    expect(document.activeElement).toBe(summary);
  });

  it("Escapeで閉じて入口へフォーカスを戻す", () => {
    const { container } = render(
      <PlaybackFilter value="all" onChange={() => undefined} />,
    );
    const summary = screen.getByText("絞り込み").closest("summary");
    if (summary === null) {
      throw new Error("絞り込みの入口がない");
    }
    fireEvent.click(summary);
    fireEvent.keyDown(document, { key: "Escape" });

    expect(container.querySelector("details")?.hasAttribute("open")).toBe(false);
    expect(document.activeElement).toBe(summary);
  });
});
