import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useTheaterMode } from "./theaterMode";

function stubTransition() {
  const startViewTransition = vi.fn((update: () => void) => {
    update();
  });
  Object.defineProperty(document, "startViewTransition", {
    configurable: true,
    value: startViewTransition,
  });
  return startViewTransition;
}

describe("useTheaterMode", () => {
  afterEach(() => {
    Reflect.deleteProperty(document, "startViewTransition");
    vi.unstubAllGlobals();
  });

  it("切り替えを短い画面遷移の中で反映する", () => {
    const startViewTransition = stubTransition();
    const { result } = renderHook(() => useTheaterMode());
    act(() => result.current[1]());
    expect(startViewTransition).toHaveBeenCalledTimes(1);
    expect(result.current[0]).toBe(true);
  });

  it("動きを減らす設定では画面遷移を使わずに切り替える", () => {
    const startViewTransition = stubTransition();
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
    }));
    const { result } = renderHook(() => useTheaterMode());
    act(() => result.current[1]());
    expect(startViewTransition).not.toHaveBeenCalled();
    expect(result.current[0]).toBe(true);
  });
});
