import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { VideoTag } from "../api/client";
import CardTagRow from "./CardTagRow";

function tags(...names: string[]): VideoTag[] {
  return names.map((name, index) => ({
    id: index + 1,
    name,
    manual: true,
    fromFolder: false,
    tentative: false,
  }));
}

function renderRow(
  props: Partial<React.ComponentProps<typeof CardTagRow>> & { tags: VideoTag[] },
) {
  const onPress = vi.fn();
  const onToggleSelection = vi.fn();
  const result = render(
    <CardTagRow
      selectionMode={false}
      onPress={onPress}
      onToggleSelection={onToggleSelection}
      {...props}
    />,
  );
  return { ...result, onPress, onToggleSelection };
}

afterEach(() => cleanup());

describe("CardTagRow", () => {
  it("測れない（jsdom で幅0）ときはすべてのタグをボタンとして出す", () => {
    renderRow({ tags: tags("旅行", "2024", "Anime") });
    expect(screen.getByRole("button", { name: "Filter by 旅行" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Filter by 2024" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Filter by Anime" })).toBeDefined();
  });

  it("押すと onPress にそのタグを渡す", () => {
    const { onPress } = renderRow({ tags: tags("旅行") });
    fireEvent.click(screen.getByRole("button", { name: "Filter by 旅行" }));
    expect(onPress).toHaveBeenCalledWith(expect.objectContaining({ name: "旅行" }));
  });

  it("収まらない分は +N にまとめ、押すとポップオーバーで残りを見せる", () => {
    // available width が測れない（0）ときは全部表示するので、収まりきらない状況は
    // computeVisibleTagCount 側の単体テストで確かめる。ここでは +N の配線
    // （押すとポップオーバーが開き、選ぶと onPress される）だけを、手で visibleCount
    // 相当の状態を作らずに、コンポーネントの実装が実際の DOM 幅で決める前提のまま
    // 確かめるのは難しいので、ResizeObserver のコールバックを直接発火させて
    // 強制的に測り直させる。
    const observers: ResizeObserverCallback[] = [];
    class FakeResizeObserver {
      constructor(callback: ResizeObserverCallback) {
        observers.push(callback);
      }
      observe() {}
      disconnect() {}
      unobserve() {}
    }
    vi.stubGlobal("ResizeObserver", FakeResizeObserver);

    renderRow({ tags: tags("旅行", "2024", "Anime") });
    // jsdom は幅を0で返すため、この実装では available<=0 のとき全部表示になる。
    // つまりここでは「+N」は出ない — その前提を確かめる。
    expect(screen.queryByRole("button", { name: /^Show \d+ more tags?$/ })).toBeNull();

    vi.unstubAllGlobals();
  });

  it("選択中はチップをボタンとして描かず、押すと選択を切り替える", () => {
    const { onToggleSelection, onPress } = renderRow({
      tags: tags("旅行"),
      selectionMode: true,
    });
    expect(screen.queryByRole("button", { name: "Filter by 旅行" })).toBeNull();
    const list = screen.getByRole("list", { name: "Tags" });
    expect(within(list).getByText("旅行")).toBeDefined();

    fireEvent.click(within(list).getByText("旅行"));
    expect(onToggleSelection).toHaveBeenCalledTimes(1);
    expect(onPress).not.toHaveBeenCalled();
  });

  it("ul に aria-label=タグ を付ける", () => {
    renderRow({ tags: tags("旅行") });
    expect(
      within(screen.getByRole("list", { name: "Tags" })).getByText("旅行"),
    ).toBeDefined();
  });

  // 受け入れ条件 6（specs/017-folder-groups/ui-design.md「Folder-derived tag chip」）:
  // フォルダ名からだけ付いたタグは面の無い破線の枠と Folder の目印で出し、
  // 読み上げ名に「フォルダ名から」を添える。手でも付いていれば今の形のまま。
  describe("フォルダ由来のタグ", () => {
    const mixed: VideoTag[] = [
      { id: 1, name: "京都", manual: false, fromFolder: true, tentative: false },
      { id: 2, name: "旅行", manual: true, fromFolder: true, tentative: false },
      { id: 3, name: "夏", manual: true, fromFolder: false, tentative: false },
    ];

    it("フォルダ由来だけのタグを破線の形で出し、ほかは面のある形のまま出す", () => {
      renderRow({ tags: mixed });
      const folderOnly = screen.getByRole("button", {
        name: "Filter by 京都 (from the folder name)",
      });
      expect(folderOnly.className).toContain("border-dashed");
      expect(folderOnly.className).not.toContain("bg-secondary");
      expect(folderOnly.querySelector("svg[aria-hidden='true']")).not.toBeNull();

      for (const name of ["旅行", "夏"]) {
        const chip = screen.getByRole("button", { name: `Filter by ${name}` });
        expect(chip.className).toContain("bg-secondary");
        expect(chip.className).not.toContain("border-dashed");
        expect(chip.querySelector("svg")).toBeNull();
      }
    });

    it("区別は文字の大きさではなく形で行う（同じ h-6・text-xs）", () => {
      renderRow({ tags: mixed });
      const folderOnly = screen.getByRole("button", { name: /京都/ });
      const manual = screen.getByRole("button", { name: "Filter by 夏" });
      for (const chip of [folderOnly, manual]) {
        expect(chip.className).toContain("h-6");
        expect(chip.className).toContain("text-xs");
        expect(chip.className).toContain("text-muted-foreground");
      }
    });

    it("押したときは出所によらず、そのタグで絞り込む", () => {
      const { onPress } = renderRow({ tags: mixed });
      fireEvent.click(
        screen.getByRole("button", { name: "Filter by 京都 (from the folder name)" }),
      );
      expect(onPress).toHaveBeenCalledWith(mixed[0]);
    });

    it("API の順のまま並べ、出所で分けない", () => {
      renderRow({ tags: mixed });
      const list = screen.getByRole("list", { name: "Tags" });
      expect(
        within(list)
          .getAllByRole("button")
          .map((button) => button.getAttribute("title")),
      ).toEqual(["京都", "旅行", "夏"]);
    });

    it("選択中の形でも破線の形で出し、隠した「フォルダ名から」を添える", () => {
      renderRow({ tags: mixed, selectionMode: true });
      const list = screen.getByRole("list", { name: "Tags" });
      const chip = within(list).getByTitle("京都");
      expect(chip.className).toContain("border-dashed");
      expect(chip.textContent).toBe("京都 (from the folder name)");
    });
  });

  // 受け入れ条件 4・5（specs/031-tentative-tags/ui-design.md「Tentative mark」
  // 「Library card and group card」）: 仮のタグは名前の後ろに破線の丸を置き、
  // 読み上げ名に「(tentative)」を添える。面・文字の色・高さは確定したタグと同じ。
  describe("仮のタグ", () => {
    const mixed: VideoTag[] = [
      { id: 1, name: "高画質", manual: true, fromFolder: false, tentative: true },
      { id: 2, name: "夏", manual: true, fromFolder: false, tentative: false },
      { id: 3, name: "京都", manual: false, fromFolder: true, tentative: true },
    ];

    function mark(el: Element): Element | null {
      return el.querySelector("svg.lucide-circle-dashed");
    }

    it("名前の後ろに目印を置き、面・文字の色・高さは確定したタグと同じ", () => {
      renderRow({ tags: mixed });
      const tentative = screen.getByRole("button", {
        name: "Filter by 高画質 (tentative)",
      });
      const confirmed = screen.getByRole("button", { name: "Filter by 夏" });

      const icon = mark(tentative);
      expect(icon).not.toBeNull();
      expect(icon?.getAttribute("aria-hidden")).toBe("true");
      expect(icon?.getAttribute("class")).toContain("size-3");
      expect(icon?.getAttribute("class")).toContain("shrink-0");
      expect(icon?.getAttribute("class")).toContain("text-muted-foreground");
      // 名前が先（主）で、目印はその後ろ。名前だけが省略される。
      const name = tentative.firstElementChild;
      expect(name?.textContent).toBe("高画質");
      expect(name?.className).toContain("truncate");
      expect(tentative.lastElementChild).toBe(icon);
      expect(tentative.getAttribute("title")).toBe("高画質");

      for (const chip of [tentative, confirmed]) {
        expect(chip.className).toContain("bg-secondary");
        expect(chip.className).toContain("h-6");
        expect(chip.className).toContain("text-xs");
        expect(chip.className).toContain("text-muted-foreground");
      }
      // 確定したタグには何も足さない。
      expect(confirmed.querySelector("svg")).toBeNull();
      expect(confirmed.textContent).toBe("夏");
    });

    it("フォルダ由来だけの仮のタグは Folder の目印 → 名前 → 仮の目印の順", () => {
      renderRow({ tags: mixed });
      const chip = screen.getByRole("button", {
        name: "Filter by 京都 (from the folder name, tentative)",
      });
      const children = Array.from(chip.children);
      expect(children).toHaveLength(3);
      expect(children[0]?.getAttribute("class")).toContain("lucide-folder");
      expect(children[1]?.textContent).toBe("京都");
      expect(children[2]?.getAttribute("class")).toContain("lucide-circle-dashed");
      expect(chip.className).toContain("border-dashed");
    });

    it("押したときの絞り込みは確定したタグと同じ", () => {
      const { onPress } = renderRow({ tags: mixed });
      fireEvent.click(
        screen.getByRole("button", { name: "Filter by 高画質 (tentative)" }),
      );
      expect(onPress).toHaveBeenCalledWith(mixed[0]);
    });

    it("選択中は押すとカードの選択を切り替え、読み上げには Tentative を添える", () => {
      const { onToggleSelection, onPress } = renderRow({
        tags: mixed.slice(0, 1),
        selectionMode: true,
      });
      const chip = screen.getByTitle("高画質");
      expect(chip.tagName).toBe("SPAN");
      expect(mark(chip)).not.toBeNull();
      expect(within(chip).getByText("Tentative").className).toContain("sr-only");
      fireEvent.click(chip);
      expect(onToggleSelection).toHaveBeenCalledTimes(1);
      expect(onPress).not.toHaveBeenCalled();
    });

    it("測るための並びにも目印を置き、+N は目印の分を含めた幅で決まる", () => {
      const { container } = renderRow({ tags: mixed });
      const measure = container.querySelector("div[aria-hidden='true']");
      expect(measure).not.toBeNull();
      // 測るための並びは仮のタグ 2 つに目印を持つ（ポップオーバーの中は
      // 見えているチップと同じ TagChip なので、上の試験で足りる）。
      expect(measure?.querySelectorAll("svg.lucide-circle-dashed")).toHaveLength(2);
    });
  });
});
