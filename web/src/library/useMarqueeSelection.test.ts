import { renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import type { LibraryItem } from "../api/client";
import { emptySelection } from "./selection";
import { useMarqueeSelection } from "./useMarqueeSelection";

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

test("unmounting during a drag stops tracking the pointer", () => {
  const root = document.createElement("div");
  const box = document.createElement("div");
  document.body.append(root, box);
  const setSelection = vi.fn();
  const frame = vi.spyOn(window, "requestAnimationFrame");
  const { unmount } = renderHook(() => {
    useMarqueeSelection({
      container: { current: root },
      box: { current: box },
      enabled: true,
      items: { current: [] as readonly LibraryItem[] },
      selection: emptySelection,
      setSelection,
    });
  });

  root.dispatchEvent(
    new PointerEvent("pointerdown", {
      pointerType: "mouse",
      isPrimary: true,
      button: 0,
      clientX: 0,
      clientY: 0,
      bubbles: true,
      cancelable: true,
    }),
  );
  unmount();
  window.dispatchEvent(
    new PointerEvent("pointermove", { pointerType: "mouse", clientX: 50, clientY: 50 }),
  );

  expect(frame).not.toHaveBeenCalled();
  expect(setSelection).not.toHaveBeenCalled();
});
