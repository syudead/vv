import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MAX_QUERY_LENGTH } from "../api/client";
import SearchBox, { limitQueryInput } from "./SearchBox";

function renderBox(query = "京都") {
  const onCommit = vi.fn();
  render(<SearchBox query={query} onCommit={onCommit} />);
  return {
    onCommit,
    box: screen.getByRole("searchbox", { name: "Search videos" }) as HTMLInputElement,
    help: screen.getByRole("button", { name: "How to search" }),
  };
}

describe("SearchBox の検索の書き方", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("開くと4つの書き方を例つきで出し、フォーカスはボタンに残る", async () => {
    const user = userEvent.setup();
    const { help } = renderBox();
    expect(help.getAttribute("aria-expanded")).toBe("false");
    expect(help.hasAttribute("aria-describedby")).toBe(false);

    await user.click(help);

    const dialog = await screen.findByRole("dialog", { name: "How to search" });
    expect(within(dialog).getByRole("heading", { name: "How to search" })).toBeDefined();
    const examples = within(dialog)
      .getAllByRole("term")
      .map((term) => Array.from(term.children).map((line) => line.textContent));
    expect(examples).toEqual([
      ["kyoto 2024"],
      ['"kyoto trip 2024"'],
      ["kyoto -2023"],
      ["kyoto OR nara", "kyoto | nara"],
    ]);
    expect(
      within(dialog)
        .getAllByRole("definition")
        .map((meaning) => meaning.textContent),
    ).toEqual([
      "Finds videos that contain every space-separated word",
      'Words wrapped in " are searched for as one term, spaces included',
      "Leaves out videos that contain a word starting with -",
      "Finds videos that contain either word. Binds tighter than a space",
    ]);
    expect(
      within(dialog).getByText(
        "Full-width and half-width characters, upper and lower case, and hiragana and katakana are treated the same.",
      ),
    ).toBeDefined();
    expect(within(dialog).getByText("Only the first 16 terms are used.")).toBeDefined();

    expect(document.activeElement).toBe(help);
    expect(help.getAttribute("aria-expanded")).toBe("true");
    expect(help.getAttribute("aria-controls")).toBe(dialog.id);
    const described = document.getElementById(
      help.getAttribute("aria-describedby") ?? "",
    );
    expect(described).not.toBeNull();
    expect(dialog.contains(described)).toBe(true);
    expect(described?.textContent).not.toContain("How to search");
  });

  it("Enter と Space で開き、フォーカスだけでは開かない", async () => {
    const user = userEvent.setup();
    const { help } = renderBox();
    help.focus();
    expect(screen.queryByRole("dialog")).toBeNull();

    await user.keyboard("{Enter}");
    expect(await screen.findByRole("dialog")).toBeDefined();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.keyboard(" ");
    expect(await screen.findByRole("dialog")).toBeDefined();
  });

  it("Esc はポップオーバーだけを閉じ、検索語とボタンのフォーカスを残す", async () => {
    const user = userEvent.setup();
    const { box, help, onCommit } = renderBox();

    await user.click(help);
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(box.value).toBe("京都");
    expect(document.activeElement).toBe(help);
    expect(help.getAttribute("aria-expanded")).toBe("false");
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("もう一度押して閉じても検索語は残り、例を押しても検索欄に入らない", async () => {
    const user = userEvent.setup();
    const { box, help, onCommit } = renderBox();

    await user.click(help);
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByText("kyoto -2023"));
    expect(box.value).toBe("京都");

    await user.click(help);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(box.value).toBe("京都");
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("検索欄での Esc は今のまま検索語を消して抜ける", async () => {
    const user = userEvent.setup();
    const { box, onCommit } = renderBox();

    await user.click(box);
    await user.keyboard("{Escape}");

    expect(box.value).toBe("");
    expect(document.activeElement).not.toBe(box);
    expect(onCommit).toHaveBeenCalledWith("", "push");
  });

  it("検索語があるときは × の右に並び、Tab は検索欄 → × → 手引きの順に進む", async () => {
    const user = userEvent.setup();
    const { box, help } = renderBox();

    await user.click(box);
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Clear search" }),
    );
    await user.tab();
    expect(document.activeElement).toBe(help);
  });
});

describe("SearchBox の入力", () => {
  afterEach(() => cleanup());

  it("検索語の上限は符号位置で数える（絵文字も 100 個まで入る）", () => {
    const emoji = "😀".repeat(MAX_QUERY_LENGTH + 1);
    expect(Array.from(limitQueryInput(emoji))).toHaveLength(MAX_QUERY_LENGTH);
    expect(limitQueryInput("😀".repeat(MAX_QUERY_LENGTH))).toBe(
      "😀".repeat(MAX_QUERY_LENGTH),
    );
    // 上限に達した語の先頭や途中に打っても、もとの末尾は消えない。
    const full = "a".repeat(MAX_QUERY_LENGTH);
    expect(limitQueryInput("X" + full, full)).toBe(full);
    expect(limitQueryInput("a".repeat(50) + "X" + "a".repeat(50), full)).toBe(full);
    // 上限の語の一文字を選び、その文字で終わる語に置き換えても、入る分だけが入る。
    expect(limitQueryInput("ba" + "a".repeat(MAX_QUERY_LENGTH - 1), full, 2)).toBe(
      "b" + "a".repeat(MAX_QUERY_LENGTH - 1),
    );
    expect(limitQueryInput("X" + full, full, 1)).toBe(full);
    expect(limitQueryInput("a".repeat(50) + "X" + "a".repeat(50), full, 51)).toBe(full);
    // 上限の手前での貼り付けは、入る分だけが入る（絵文字を割らない）。
    const almost = "b".repeat(MAX_QUERY_LENGTH - 2);
    expect(
      limitQueryInput("b" + "😀😀😀" + "b".repeat(MAX_QUERY_LENGTH - 3), almost),
    ).toBe("b" + "😀😀" + "b".repeat(MAX_QUERY_LENGTH - 3));

    render(<SearchBox query="" onCommit={() => {}} />);
    const input = screen.getByRole<HTMLInputElement>("searchbox", {
      name: "Search videos",
    });
    expect(input.maxLength).toBe(-1);
    fireEvent.change(input, { target: { value: emoji } });
    expect(Array.from(input.value)).toHaveLength(MAX_QUERY_LENGTH);
  });

  it("入力途中に × を押しても、履歴を増やす確定は起きない", () => {
    const onCommit = vi.fn();
    render(<SearchBox query="" onCommit={onCommit} />);
    const input = screen.getByRole<HTMLInputElement>("searchbox", {
      name: "Search videos",
    });
    input.focus();
    fireEvent.change(input, { target: { value: "京都" } });

    const clear = screen.getByRole("button", { name: "Clear search" });
    // × の mousedown は入力欄のフォーカスを外さない（blur で入力途中の語を確定しない）。
    expect(fireEvent.mouseDown(clear)).toBe(false);
    fireEvent.click(clear);

    expect(onCommit).not.toHaveBeenCalled();
    expect(input.value).toBe("");
  });
});
