import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TagRef } from "../api/client";
import CardTagRow from "./CardTagRow";
import { TagRowMeasureProvider } from "./TagRowMeasure";

function tags(...names: string[]): TagRef[] {
  return names.map((name, index) => ({ id: index + 1, name }));
}

afterEach(() => cleanup());

describe("TagRowMeasureProvider（B2）", () => {
  it("複数の CardTagRow がいても、ResizeObserver は1つだけ作る", () => {
    const instances: unknown[] = [];
    class FakeResizeObserver {
      constructor() {
        instances.push(this);
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    }
    vi.stubGlobal("ResizeObserver", FakeResizeObserver);

    render(
      <TagRowMeasureProvider>
        <CardTagRow
          tags={tags("旅行")}
          selectionMode={false}
          onPress={vi.fn()}
          onToggleSelection={vi.fn()}
        />
        <CardTagRow
          tags={tags("2024")}
          selectionMode={false}
          onPress={vi.fn()}
          onToggleSelection={vi.fn()}
        />
        <CardTagRow
          tags={tags("Anime")}
          selectionMode={false}
          onPress={vi.fn()}
          onToggleSelection={vi.fn()}
        />
      </TagRowMeasureProvider>,
    );

    expect(screen.getAllByRole("list", { name: "タグ" })).toHaveLength(3);
    expect(instances).toHaveLength(1);

    vi.unstubAllGlobals();
  });

  it("Provider の外（単体テストなど）では、タグは普通に描画される", () => {
    render(
      <CardTagRow
        tags={tags("旅行")}
        selectionMode={false}
        onPress={vi.fn()}
        onToggleSelection={vi.fn()}
      />,
    );
    expect(screen.getByRole("list", { name: "タグ" })).toBeDefined();
  });
});
