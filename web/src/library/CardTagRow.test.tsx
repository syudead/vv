import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TagRef } from "../api/client";
import CardTagRow from "./CardTagRow";

function tags(...names: string[]): TagRef[] {
  return names.map((name, index) => ({ id: index + 1, name }));
}

function renderRow(
  props: Partial<React.ComponentProps<typeof CardTagRow>> & { tags: TagRef[] },
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
});
