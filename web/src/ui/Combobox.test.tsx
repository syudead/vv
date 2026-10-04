import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import Combobox, { nameReason } from "./Combobox";

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

describe("nameReason の制御文字", () => {
  it.each([
    ["U+0000", "a\u0000b"],
    ["U+001F", "a\u001fb"],
    ["U+007F", "a\u007fb"],
    ["U+0085", "a\u0085b"],
    ["U+009F", "a\u009fb"],
  ])("%s を含む名前を拒む", (_, name) => {
    expect(nameReason(name)).toBe(t.tagName.controlCharacters);
  });

  it.each([
    ["U+00A0", "a b"],
    ["U+200B", "a​b"],
    ["U+0020", "a b"],
  ])("制御文字でない %s は拒まない", (_, name) => {
    expect(nameReason(name)).toBeNull();
  });
});

describe("Combobox の候補の一覧の幅", () => {
  const options = [{ id: "1", label: "Anime" }];

  it("指定しなければ枠と別の w-64 で開き、listClassName を渡すとそれに替わる", () => {
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
    expect(screen.getByRole("listbox").className).toContain("w-64");

    rerender(view("w-full"));
    expect(screen.getByRole("listbox").className).toContain("w-full");
    expect(screen.getByRole("listbox").className).not.toContain("w-64");
  });
});

describe("Combobox の候補の一覧は見えている行だけを描く", () => {
  const rowHeight = 32;
  const many = Array.from({ length: 3000 }, (_, index) => ({
    id: String(index + 1),
    label: `Tag ${String(index + 1)}`,
  }));
  let originalOffsetHeight: PropertyDescriptor | undefined;
  let originalScrollTo: PropertyDescriptor | undefined;
  let originalScrollHeight: PropertyDescriptor | undefined;

  beforeEach(() => {
    originalOffsetHeight = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetHeight",
    );
    originalScrollTo = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollTo");
    originalScrollHeight = Object.getOwnPropertyDescriptor(
      Element.prototype,
      "scrollHeight",
    );
    // スクロールできる長さ（全行 + 上下の余白）。clientHeight は jsdom では 0 なので、
    // 箱の高さを引いた分だけ足りないが、行 49 までの位置には十分である。
    Object.defineProperty(Element.prototype, "scrollHeight", {
      configurable: true,
      get(this: Element) {
        return this.getAttribute("role") === "listbox" ? 3000 * rowHeight + 8 : 0;
      },
    });
    // 一覧の箱（max-h-64・h-60）と行の高さを jsdom に与える。
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get(this: HTMLElement) {
        if (this.hasAttribute("data-index")) return rowHeight;
        if (this.getAttribute("role") === "listbox") return 256;
        if (this.className.includes("h-60")) return 240;
        return 0;
      },
    });
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      writable: true,
      value: vi.fn(),
    });
  });

  afterEach(() => {
    if (originalOffsetHeight !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", originalOffsetHeight);
    }
    if (originalScrollHeight !== undefined) {
      Object.defineProperty(Element.prototype, "scrollHeight", originalScrollHeight);
    }
    if (originalScrollTo !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "scrollTo", originalScrollTo);
    } else {
      delete (HTMLElement.prototype as { scrollTo?: unknown }).scrollTo;
    }
  });

  it("3000 件でも描く行は見える分と前後の数行だけで、各行は集合の中の位置を持つ", () => {
    render(
      <Combobox
        value=""
        onValueChange={() => {}}
        options={many}
        onSelect={() => {}}
        aria-label={t.tags.create.label}
      />,
    );
    act(() => screen.getByRole("combobox").focus());
    const rows = screen.getAllByRole("option");
    expect(rows.length).toBeGreaterThan(5);
    expect(rows.length).toBeLessThan(30);
    rows.forEach((row, index) => {
      expect(row.id).toMatch(new RegExp(`-option-${String(index)}$`));
      expect(row.getAttribute("aria-setsize")).toBe("3000");
      expect(row.getAttribute("aria-posinset")).toBe(String(index + 1));
    });
  });

  it("矢印で選んだ行は範囲の外でも描き、そこへスクロールし、aria-activedescendant が指す", () => {
    render(
      <Combobox
        value=""
        onValueChange={() => {}}
        options={many}
        onSelect={() => {}}
        aria-label={t.tags.create.label}
      />,
    );
    const combo = screen.getByRole("combobox");
    act(() => combo.focus());
    for (let i = 0; i < 50; i++) fireEvent.keyDown(combo, { key: "ArrowDown" });

    const activeId = combo.getAttribute("aria-activedescendant")!;
    expect(activeId).toMatch(/-option-49$/);
    const active = document.getElementById(activeId)!;
    expect(active).not.toBeNull();
    expect(active.getAttribute("aria-selected")).toBe("true");
    expect(active.getAttribute("aria-posinset")).toBe("50");
    expect(active.textContent).toBe("Tag 50");

    const listbox = screen.getByRole("listbox");
    const scrollTo = vi.mocked(listbox.scrollTo);
    const lastTop = (scrollTo.mock.calls.at(-1)?.[0] as ScrollToOptions | undefined)?.top;
    // 行 49 の下端（上の余白 4 + 50 行）が箱の高さ 256 に収まる位置。
    expect(lastTop).toBe(4 + 50 * rowHeight - 256);
  });

  it("inline の一覧でも作成の行は末尾で、集合の大きさに数える", () => {
    render(
      <Combobox
        value="new"
        onValueChange={() => {}}
        options={many.slice(0, 3)}
        onSelect={() => {}}
        createLabel="Create new"
        aria-label={t.tags.create.label}
        inline
      />,
    );
    const rows = screen.getAllByRole("option");
    expect(rows.map((row) => row.textContent)).toEqual([
      "Tag 1",
      "Tag 2",
      "Tag 3",
      "Create new",
    ]);
    expect(rows.map((row) => row.getAttribute("aria-posinset"))).toEqual([
      "1",
      "2",
      "3",
      "4",
    ]);
    expect(rows.every((row) => row.getAttribute("aria-setsize") === "4")).toBe(true);
  });
});
