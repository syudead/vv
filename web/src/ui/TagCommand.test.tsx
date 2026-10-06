import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import TagCommand, { type TagChoice, type TagCommandLayout } from "./TagCommand";

const choices: TagChoice[] = [
  { id: "1", label: "Anime" },
  { id: "2", label: "Drama" },
];

function Harness({
  layout,
  onSelect = () => {},
  onCreate = () => {},
  onEscape,
  initial = "",
}: {
  layout: TagCommandLayout;
  onSelect?: (choice: TagChoice) => void;
  onCreate?: (spelling: string) => void;
  onEscape?: () => void;
  initial?: string;
}) {
  const [value, setValue] = useState(initial);
  return (
    <TagCommand
      layout={layout}
      label={t.player.tags.add}
      value={value}
      onValueChange={setValue}
      choices={choices}
      exactChoice={null}
      onSelect={onSelect}
      createLabel={value.trim() === "" ? null : t.player.tags.create(value.trim())}
      onCreate={onCreate}
      icon={null}
      busy={false}
      onEscape={onEscape}
    />
  );
}

function input() {
  return screen.getByRole("combobox", { name: t.player.tags.add });
}

describe("TagCommand（dropdown）", () => {
  it("入力に触れると開き、矢印を押すまでどの行も選ばない", async () => {
    render(<Harness layout="dropdown" />);
    expect(input().getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("listbox")).toBeNull();

    act(() => input().focus());
    expect(input().getAttribute("aria-expanded")).toBe("true");
    const listbox = screen.getByRole("listbox");
    expect(listbox.className).toContain("w-combobox-list");
    expect(input().getAttribute("aria-activedescendant")).toBeNull();
    for (const option of screen.getAllByRole("option")) {
      expect(option.getAttribute("aria-selected")).toBe("false");
    }

    fireEvent.keyDown(input(), { key: "ArrowDown" });
    const first = screen.getByRole("option", { name: "Anime" });
    expect(first.getAttribute("aria-selected")).toBe("true");
    expect(input().getAttribute("aria-activedescendant")).toBe(first.id);

    fireEvent.keyDown(input(), { key: "ArrowDown" });
    const second = screen.getByRole("option", { name: "Drama" });
    expect(second.getAttribute("aria-selected")).toBe("true");
    expect(first.getAttribute("aria-selected")).toBe("false");
    expect(input().getAttribute("aria-activedescendant")).toBe(second.id);
  });

  it("作成の行は末尾に置き、矢印で選ばない Enter は綴りで作る", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn();
    const onSelect = vi.fn();
    render(<Harness layout="dropdown" onCreate={onCreate} onSelect={onSelect} />);
    await user.click(input());
    await user.type(input(), "a");

    const options = screen.getAllByRole("option");
    expect(options.at(-1)?.textContent).toBe(t.player.tags.create("a"));
    await user.keyboard("{Enter}");
    expect(onCreate).toHaveBeenCalledWith("a");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("Esc は開いている一覧を閉じ、閉じているときは onEscape を呼ぶ。打ち直すと開き直す", async () => {
    const user = userEvent.setup();
    const onEscape = vi.fn();
    render(<Harness layout="dropdown" onEscape={onEscape} />);
    await user.click(input());
    expect(screen.getByRole("listbox")).toBeDefined();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input().getAttribute("aria-expanded")).toBe("false");
    expect(onEscape).not.toHaveBeenCalled();

    await user.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledTimes(1);

    await user.type(input(), "x");
    expect(screen.getByRole("listbox")).toBeDefined();
  });

  it("Tab とフォーカスが外れると一覧を閉じる", async () => {
    const user = userEvent.setup();
    render(<Harness layout="dropdown" />);
    await user.click(input());
    expect(screen.getByRole("listbox")).toBeDefined();
    await user.tab();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("閉じているときの下矢印は開いて先頭の行を選ぶ", () => {
    render(<Harness layout="dropdown" />);
    act(() => input().focus());
    fireEvent.keyDown(input(), { key: "Escape" });
    expect(screen.queryByRole("listbox")).toBeNull();

    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(
      screen.getByRole("option", { name: "Anime" }).getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("Home と End は入力のカーソルに任せ、行を選ばない", () => {
    render(<Harness layout="dropdown" initial="ab" />);
    act(() => input().focus());
    const end = fireEvent.keyDown(input(), { key: "End" });
    const home = fireEvent.keyDown(input(), { key: "Home" });
    // fireEvent は既定の動作が止められなかったときに true を返す。
    expect(end).toBe(true);
    expect(home).toBe(true);
    expect(input().getAttribute("aria-activedescendant")).toBeNull();
  });

  it("行の上にポインターを置いてからの Enter は、その行を選ぶ", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<Harness layout="dropdown" onSelect={onSelect} />);
    await user.click(input());
    fireEvent.mouseEnter(screen.getByRole("option", { name: "Drama" }));
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith(choices[1]);
  });
});

describe("TagCommand（inline）", () => {
  it("一覧は触れる前から開いていて、Esc でも閉じない", async () => {
    const user = userEvent.setup();
    render(<Harness layout="inline" />);
    expect(input().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("listbox").className).not.toContain("absolute");

    await user.click(input());
    await user.keyboard("{Escape}");
    expect(screen.getByRole("listbox")).toBeDefined();
    expect(input().getAttribute("aria-expanded")).toBe("true");
  });
});
