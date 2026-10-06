import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useScrollTopOnChange } from "./useScrollTopOnChange";

describe("useScrollTopOnChange", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("最初の描画では動かず、条件が変わったときだけ先頭へ戻す", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    const { rerender } = renderHook(({ signature }) => useScrollTopOnChange(signature), {
      initialProps: { signature: "q=a" },
    });
    expect(scrollTo).not.toHaveBeenCalled();

    rerender({ signature: "q=a" });
    expect(scrollTo).not.toHaveBeenCalled();

    rerender({ signature: "q=a&tag=1" });
    expect(scrollTo).toHaveBeenCalledTimes(1);
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: "auto" });
  });
});
