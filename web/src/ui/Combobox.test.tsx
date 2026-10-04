import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import Combobox from "./Combobox";

describe("Combobox の open の通知", () => {
  it("開閉が変わったときだけ呼び、毎回新しい関数を渡されても呼び直さない", () => {
    const calls: boolean[] = [];
    const view = (value: string) => (
      <Combobox
        value={value}
        onValueChange={() => {}}
        options={[]}
        onSelect={() => {}}
        aria-label={t.tags.create.label}
        // 呼び出し元が描画のたびに新しい関数を渡す場合を再現する。
        onOpenChange={(open) => calls.push(open)}
      />
    );
    const { rerender } = render(view(""));
    expect(calls).toEqual([false]);

    rerender(view("a"));
    rerender(view("ab"));
    expect(calls).toEqual([false]);

    act(() => screen.getByRole("combobox").focus());
    expect(calls).toEqual([false, true]);
    rerender(view("abc"));
    expect(calls).toEqual([false, true]);

    act(() => screen.getByRole("combobox").blur());
    expect(calls).toEqual([false, true, false]);
  });

  it("通知するのは描画し直したあとの最新の関数である", () => {
    const first = vi.fn();
    const latest = vi.fn();
    const view = (onOpenChange: (open: boolean) => void) => (
      <Combobox
        value=""
        onValueChange={() => {}}
        options={[]}
        onSelect={() => {}}
        aria-label={t.tags.create.label}
        onOpenChange={onOpenChange}
      />
    );
    const { rerender } = render(view(first));
    rerender(view(latest));
    fireEvent.focus(screen.getByRole("combobox"));
    expect(first).toHaveBeenCalledTimes(1);
    expect(latest).toHaveBeenCalledWith(true);
  });
});

describe("Combobox の候補の一覧の幅", () => {
  const options = [{ id: "1", label: "Anime" }];

  it("指定しなければ枠と別の w-combobox-list で開き、listClassName を渡すとそれに替わる", () => {
    const view = (listClassName?: string) => (
      <Combobox
        value=""
        onValueChange={() => {}}
        options={options}
        onSelect={() => {}}
        aria-label={t.tags.create.label}
        listClassName={listClassName}
      />
    );
    const { rerender } = render(view());
    act(() => screen.getByRole("combobox").focus());
    expect(screen.getByRole("listbox").className).toContain("w-combobox-list");

    rerender(view("w-full"));
    expect(screen.getByRole("listbox").className).toContain("w-full");
    expect(screen.getByRole("listbox").className).not.toContain("w-combobox-list");
  });
});
