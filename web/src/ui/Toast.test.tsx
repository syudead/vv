import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, useToast } from "./Toast";

function Harness() {
  const toast = useToast();
  return (
    <>
      {(["first", "second", "third"] as const).map((message) => (
        <button key={message} type="button" onClick={() => toast(message)}>
          {message}
        </button>
      ))}
    </>
  );
}

describe("ToastProvider", () => {
  afterEach(() => vi.useRealTimers());

  it("shows queued toasts one at a time on playback without dropping them", () => {
    vi.useFakeTimers();
    const { rerender } = render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    const liveRegion = document.querySelector('[aria-live="polite"]');
    expect(liveRegion).not.toBeNull();
    const toasts = within(liveRegion!);
    for (const message of ["first", "second", "third"]) {
      fireEvent.click(screen.getByRole("button", { name: message }));
      expect(toasts.getByText(message)).toBeDefined();
    }

    rerender(
      <ToastProvider placement="playback">
        <Harness />
      </ToastProvider>,
    );
    expect(toasts.getByText("first")).toBeDefined();
    expect(toasts.queryByText("second")).toBeNull();
    expect(toasts.queryByText("third")).toBeNull();

    act(() => vi.advanceTimersByTime(2800));
    expect(toasts.queryByText("first")).toBeNull();
    expect(toasts.getByText("second")).toBeDefined();

    act(() => vi.advanceTimersByTime(2800));
    expect(toasts.queryByText("second")).toBeNull();
    expect(toasts.getByText("third")).toBeDefined();

    act(() => vi.advanceTimersByTime(2800));
    expect(toasts.queryByText("third")).toBeNull();
  });
});
