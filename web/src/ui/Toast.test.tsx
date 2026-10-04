import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UiText } from "../i18n";
import { ToastProvider, toastDuration, useToast } from "./Toast";

function Harness() {
  const toast = useToast();
  return (
    <>
      {(["first", "second", "third", "fourth"] as const).map((message) => (
        <button key={message} type="button" onClick={() => toast(message as UiText)}>
          {message}
        </button>
      ))}
    </>
  );
}

// Sonner は通知を次の tick で描き、消すときは 200ms の退場の後に外す。
const settle = 250;

function press(message: string) {
  fireEvent.click(screen.getByRole("button", { name: message }));
}

/** visible は今出ている（退場中でない）通知の文言である。 */
function visible(): string[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>(
      "[data-sonner-toast]:not([data-removed=true])",
    ),
  ).map((toast) => toast.querySelector("[data-title]")?.textContent ?? "");
}

function advance(ms: number) {
  act(() => vi.advanceTimersByTime(ms));
}

describe("ToastProvider", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  it("names the notification region from the catalog", () => {
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    expect(screen.getByRole("region", { name: "Notifications" })).toBeDefined();
  });

  it("shows queued toasts one at a time on playback without dropping them", () => {
    const { rerender } = render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    for (const message of ["first", "second", "third"]) press(message);
    advance(10);
    expect(visible().sort()).toEqual(["first", "second", "third"]);

    rerender(
      <ToastProvider placement="playback">
        <Harness />
      </ToastProvider>,
    );
    advance(settle);
    expect(visible()).toEqual(["first"]);
    press("fourth");
    advance(10);
    expect(visible()).toEqual(["first"]);

    for (const message of ["second", "third", "fourth"]) {
      advance(toastDuration);
      advance(10);
      expect(visible()).toEqual([message]);
    }
    advance(toastDuration + settle);
    expect(visible()).toEqual([]);
  });

  it("shows a new default toast immediately among the latest three", () => {
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    for (const message of ["first", "second", "third", "fourth"]) press(message);
    advance(settle);

    expect(visible().sort()).toEqual(["fourth", "second", "third"]);
  });

  it("keeps the visible toast's remaining time across placement changes", () => {
    const { rerender } = render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    press("first");
    advance(10);
    advance(toastDuration - 300);

    rerender(
      <ToastProvider placement="playback">
        <Harness />
      </ToastProvider>,
    );
    advance(200);
    expect(visible()).toEqual(["first"]);

    advance(200);
    expect(visible()).toEqual([]);
  });

  it("shows at most three queued toasts after leaving playback", () => {
    const { rerender } = render(
      <ToastProvider placement="playback">
        <Harness />
      </ToastProvider>,
    );
    for (const message of ["first", "second", "third", "fourth"]) press(message);
    advance(10);
    expect(visible()).toEqual(["first"]);

    rerender(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    advance(10);
    expect(visible().sort()).toEqual(["first", "second", "third"]);

    advance(toastDuration);
    advance(10);
    expect(visible()).toContain("fourth");
  });
});
