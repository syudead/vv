import { act, renderHook } from "@testing-library/react";
import type { PointerEvent } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useHoverOpen } from "./useHoverOpen";

function pointer(pointerType: string): PointerEvent<HTMLElement> {
  return { pointerType } as PointerEvent<HTMLElement>;
}

const mouse = pointer("mouse");

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useHoverOpen", () => {
  it("マウスが「+N」に入って 400ms で開き、399ms では開かない。フォーカスを動かさない開き方になる", () => {
    const { result } = renderHook(() => useHoverOpen());
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(399));
    expect(result.current.open).toBe(false);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.open).toBe(true);
    expect(result.current.openedByHover).toBe(true);
    expect(result.current.lastOpenedByHover()).toBe(true);
  });

  it("400ms より前に離れると取り消される", () => {
    const { result } = renderHook(() => useHoverOpen());
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(100));
    act(() => result.current.triggerHandlers.onPointerLeave(mouse));
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.open).toBe(false);
  });

  it("「+N」から一覧へ移り 200ms 以内に入り直すと開いたまま", () => {
    const { result } = renderHook(() => useHoverOpen());
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(400));
    act(() => result.current.triggerHandlers.onPointerLeave(mouse));
    act(() => vi.advanceTimersByTime(199));
    act(() => result.current.contentHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.open).toBe(true);
    // 一覧から「+N」へ戻るときも同じ。
    act(() => result.current.contentHandlers.onPointerLeave(mouse));
    act(() => vi.advanceTimersByTime(150));
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.open).toBe(true);
  });

  it("両方から 200ms 離れると閉じ、199ms では閉じない", () => {
    const { result } = renderHook(() => useHoverOpen());
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(400));
    act(() => result.current.triggerHandlers.onPointerLeave(mouse));
    act(() => result.current.contentHandlers.onPointerEnter(mouse));
    act(() => result.current.contentHandlers.onPointerLeave(mouse));
    act(() => vi.advanceTimersByTime(199));
    expect(result.current.open).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.open).toBe(false);
  });

  it("touch と pen のポインタでは開かない", () => {
    const { result } = renderHook(() => useHoverOpen());
    for (const type of ["touch", "pen"]) {
      act(() => result.current.triggerHandlers.onPointerEnter(pointer(type)));
      act(() => vi.advanceTimersByTime(1000));
      expect(result.current.open).toBe(false);
    }
  });

  it("押して開いた一覧は、ポインタが離れても閉じず、フォーカスを動かす開き方になる", () => {
    const { result } = renderHook(() => useHoverOpen());
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(100));
    // 400ms を待たずに押した（クリック）。開く待ちは取り消され、押した開き方になる。
    act(() => result.current.setOpen(true));
    expect(result.current.open).toBe(true);
    expect(result.current.openedByHover).toBe(false);
    act(() => result.current.triggerHandlers.onPointerLeave(mouse));
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.open).toBe(true);
    expect(result.current.lastOpenedByHover()).toBe(false);
  });

  it("留めて開いている間に押すと閉じ、もう一度押すと押した開き方で開く", () => {
    const { result } = renderHook(() => useHoverOpen());
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(400));
    act(() => result.current.setOpen(false));
    expect(result.current.open).toBe(false);
    // 閉じた直後も、閉じた一覧は留めて開いたものだったと分かる（フォーカスを返さない）。
    expect(result.current.lastOpenedByHover()).toBe(true);
    // ポインタが「+N」に載ったままでも、勝手に開き直さない。
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.open).toBe(false);
    act(() => result.current.setOpen(true));
    expect(result.current.open).toBe(true);
    expect(result.current.openedByHover).toBe(false);
  });

  it("enabled が false になると閉じ、その間は留めても開かない", () => {
    const { result, rerender } = renderHook(({ enabled }) => useHoverOpen(enabled), {
      initialProps: { enabled: true },
    });
    act(() => result.current.setOpen(true));
    rerender({ enabled: false });
    expect(result.current.open).toBe(false);
    act(() => result.current.triggerHandlers.onPointerEnter(mouse));
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.open).toBe(false);
    rerender({ enabled: true });
    expect(result.current.open).toBe(false);
  });
});
