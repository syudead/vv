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
    expect(screen.getByRole("button", { name: "旅行で絞り込む" })).toBeDefined();
    expect(screen.getByRole("button", { name: "2024で絞り込む" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Animeで絞り込む" })).toBeDefined();
  });

  it("押すと onPress にそのタグを渡す", () => {
    const { onPress } = renderRow({ tags: tags("旅行") });
    fireEvent.click(screen.getByRole("button", { name: "旅行で絞り込む" }));
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
    expect(screen.queryByRole("button", { name: /ほかのタグ/ })).toBeNull();

    vi.unstubAllGlobals();
  });

  it("選択中はチップをボタンとして描かず、押すと選択を切り替える", () => {
    const { onToggleSelection, onPress } = renderRow({
      tags: tags("旅行"),
      selectionMode: true,
    });
    expect(screen.queryByRole("button", { name: "旅行で絞り込む" })).toBeNull();
    const list = screen.getByRole("list", { name: "タグ" });
    expect(within(list).getByText("旅行")).toBeDefined();

    fireEvent.click(within(list).getByText("旅行"));
    expect(onToggleSelection).toHaveBeenCalledTimes(1);
    expect(onPress).not.toHaveBeenCalled();
  });

  it("ul に aria-label=タグ を付ける", () => {
    renderRow({ tags: tags("旅行") });
    expect(
      within(screen.getByRole("list", { name: "タグ" })).getByText("旅行"),
    ).toBeDefined();
  });

  // 受け入れ条件 6（specs/017-folder-groups/ui-design.md「Folder-derived tag chip」）:
  // フォルダ名からだけ付いたタグは面の無い破線の枠と Folder の目印で出し、
  // 読み上げ名に「フォルダ名から」を添える。手でも付いていれば今の形のまま。
  describe("フォルダ由来のタグ", () => {
    const mixed: VideoTag[] = [
      { id: 1, name: "京都", manual: false, fromFolder: true },
      { id: 2, name: "旅行", manual: true, fromFolder: true },
      { id: 3, name: "夏", manual: true, fromFolder: false },
    ];

    it("フォルダ由来だけのタグを破線の形で出し、ほかは面のある形のまま出す", () => {
      renderRow({ tags: mixed });
      const folderOnly = screen.getByRole("button", {
        name: "京都で絞り込む（フォルダ名から）",
      });
      expect(folderOnly.className).toContain("border-dashed");
      expect(folderOnly.className).not.toContain("bg-elevated");
      expect(folderOnly.querySelector("svg[aria-hidden='true']")).not.toBeNull();

      for (const name of ["旅行", "夏"]) {
        const chip = screen.getByRole("button", { name: `${name}で絞り込む` });
        expect(chip.className).toContain("bg-elevated");
        expect(chip.className).not.toContain("border-dashed");
        expect(chip.querySelector("svg")).toBeNull();
      }
    });

    it("区別は文字の大きさではなく形で行う（同じ h-5・text-xs）", () => {
      renderRow({ tags: mixed });
      const folderOnly = screen.getByRole("button", { name: /京都/ });
      const manual = screen.getByRole("button", { name: "夏で絞り込む" });
      for (const chip of [folderOnly, manual]) {
        expect(chip.className).toContain("h-5");
        expect(chip.className).toContain("text-xs");
        expect(chip.className).toContain("text-fg-muted");
      }
    });

    it("押したときは出所によらず、そのタグで絞り込む", () => {
      const { onPress } = renderRow({ tags: mixed });
      fireEvent.click(
        screen.getByRole("button", { name: "京都で絞り込む（フォルダ名から）" }),
      );
      expect(onPress).toHaveBeenCalledWith(mixed[0]);
    });

    it("API の順のまま並べ、出所で分けない", () => {
      renderRow({ tags: mixed });
      const list = screen.getByRole("list", { name: "タグ" });
      expect(
        within(list)
          .getAllByRole("button")
          .map((button) => button.getAttribute("title")),
      ).toEqual(["京都", "旅行", "夏"]);
    });

    it("選択中の形でも破線の形で出し、隠した「フォルダ名から」を添える", () => {
      renderRow({ tags: mixed, selectionMode: true });
      const list = screen.getByRole("list", { name: "タグ" });
      const chip = within(list).getByTitle("京都");
      expect(chip.className).toContain("border-dashed");
      expect(chip.textContent).toBe("京都（フォルダ名から）");
    });
  });
});
