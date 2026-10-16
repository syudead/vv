import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ApplyTagKind } from "./useLibraryTagKinds";
import { useAddedLengthControls } from "./useAddedLengthControls";

// 追加日と長さの絞り込みを URL へ書く操作（specs/052-smart-folders/ui-design.md「Added and length」）。

describe("useAddedLengthControls", () => {
  it("選んだ追加日と長さだけを、履歴を 1 つ増やして書く", () => {
    const apply = vi.fn<ApplyTagKind>();
    const { result } = renderHook(() =>
      useAddedLengthControls({ filter: { addedWithin: 7 }, dropped: false }, apply),
    );
    expect(apply).not.toHaveBeenCalled();

    act(() => result.current.changeAdded(30));
    expect(apply).toHaveBeenLastCalledWith({ addedWithin: ["30"] }, "push");
    act(() => result.current.changeAdded(undefined));
    expect(apply).toHaveBeenLastCalledWith({ addedWithin: [] }, "push");
    act(() => result.current.changeLength({ minDuration: 3600 }));
    expect(apply).toHaveBeenLastCalledWith(
      { minDuration: ["3600"], maxDuration: [] },
      "push",
    );
  });

  it("送れない値を落としたときは、残る値で履歴を増やさずに書き直す", () => {
    const apply = vi.fn<ApplyTagKind>();
    renderHook(() =>
      useAddedLengthControls({ filter: { minDuration: 600 }, dropped: true }, apply),
    );
    expect(apply).toHaveBeenCalledExactlyOnceWith(
      { addedWithin: [], minDuration: ["600"], maxDuration: [] },
      "replace",
    );
  });
});
