import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { MAX_QUERY_LENGTH } from "../api/client";
import SearchBox, { limitQueryInput } from "./SearchBox";

describe("SearchBox", () => {
  it("検索語の上限は符号位置で数える（絵文字も 100 個まで入る）", () => {
    const emoji = "😀".repeat(MAX_QUERY_LENGTH + 1);
    expect(Array.from(limitQueryInput(emoji))).toHaveLength(MAX_QUERY_LENGTH);
    expect(limitQueryInput("😀".repeat(MAX_QUERY_LENGTH))).toBe(
      "😀".repeat(MAX_QUERY_LENGTH),
    );

    render(<SearchBox query="" onCommit={() => {}} />);
    const input = screen.getByRole<HTMLInputElement>("searchbox", { name: "動画を検索" });
    expect(input.maxLength).toBe(-1);
    fireEvent.change(input, { target: { value: emoji } });
    expect(Array.from(input.value)).toHaveLength(MAX_QUERY_LENGTH);
  });

  it("入力途中に × を押しても、履歴を増やす確定は起きない", () => {
    const onCommit = vi.fn();
    render(<SearchBox query="" onCommit={onCommit} />);
    const input = screen.getByRole<HTMLInputElement>("searchbox", { name: "動画を検索" });
    input.focus();
    fireEvent.change(input, { target: { value: "京都" } });

    const clear = screen.getByRole("button", { name: "検索語をクリア" });
    // × の mousedown は入力欄のフォーカスを外さない（blur で入力途中の語を確定しない）。
    expect(fireEvent.mouseDown(clear)).toBe(false);
    fireEvent.click(clear);

    expect(onCommit).not.toHaveBeenCalled();
    expect(input.value).toBe("");
  });
});
