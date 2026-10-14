import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UiText } from "../i18n";
import {
  actionToastDuration,
  type ToastAction,
  ToastProvider,
  toastDuration,
  useToast,
} from "./Toast";

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

/** ActionHarness はアクション付きの通知を出すボタン（「first」「second」）を描く。 */
function ActionHarness({ action }: { action: ToastAction }) {
  const toast = useToast();
  return (
    <>
      {(["first", "second"] as const).map((message) => (
        <button
          key={message}
          type="button"
          onClick={() => toast(message as UiText, { action })}
        >
          {message}
        </button>
      ))}
    </>
  );
}

/** DismissHarness は通知「first」「second」を出し、「close second」で second の閉じる関数を呼ぶ。 */
function DismissHarness() {
  const toast = useToast();
  const [closeSecond, setCloseSecond] = useState<() => void>(() => () => undefined);
  return (
    <>
      <button type="button" onClick={() => toast("first" as UiText)}>
        first
      </button>
      <button
        type="button"
        onClick={() =>
          setCloseSecond(() => toast("second" as UiText, { duration: 60_000 }))
        }
      >
        second
      </button>
      <button type="button" onClick={closeSecond}>
        close second
      </button>
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

  it("keeps the visible toast on screen across placement changes", () => {
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
    // 置き場が変わると Sonner は描き直し、その時点から出し直す。
    advance(400);
    expect(visible()).toEqual(["first"]);

    advance(toastDuration + 200);
    expect(visible()).toEqual([]);
  });

  it("closes a toast through its dismiss function after a placement change re-queued it", () => {
    const { rerender } = render(
      <ToastProvider>
        <DismissHarness />
      </ToastProvider>,
    );
    press("first");
    press("second");
    advance(10);
    expect(visible()).toEqual(["second", "first"]);

    // 再生画面では 1 件だけ出すので、second は新しい id で待ちに戻る。
    rerender(
      <ToastProvider placement="playback">
        <DismissHarness />
      </ToastProvider>,
    );
    advance(settle);
    expect(visible()).toEqual(["first"]);
    press("close second");

    advance(toastDuration + settle);
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

  describe("with an action", () => {
    const label = "Undo" as UiText;

    it("stays 8 seconds with the action at its end", () => {
      render(
        <ToastProvider>
          <ActionHarness action={{ label, run: () => Promise.resolve() }} />
        </ToastProvider>,
      );
      press("first");
      advance(10);
      expect(screen.getByRole("button", { name: "Undo" })).toBeDefined();

      advance(toastDuration + settle);
      expect(visible()).toEqual(["first"]);
      advance(actionToastDuration - toastDuration + settle);
      expect(visible()).toEqual([]);
    });

    it("replaces the previous toast with an action, so one action shows", () => {
      render(
        <ToastProvider>
          <ActionHarness action={{ label, run: () => Promise.resolve() }} />
        </ToastProvider>,
      );
      press("first");
      advance(10);
      press("second");
      advance(settle);
      expect(visible()).toEqual(["second"]);
      advance(settle);
      expect(screen.getAllByRole("button", { name: "Undo" })).toHaveLength(1);
    });

    it("disables the action while it runs and closes the toast when it ends", async () => {
      let finish: () => void = () => undefined;
      const run = vi.fn(
        () =>
          new Promise<void>((resolve) => {
            finish = resolve;
          }),
      );
      render(
        <ToastProvider>
          <ActionHarness action={{ label, run }} />
        </ToastProvider>,
      );
      press("first");
      advance(10);
      const action = screen.getByRole("button", { name: "Undo" });
      fireEvent.click(action);
      fireEvent.click(action);

      expect(run).toHaveBeenCalledTimes(1);
      expect((action as HTMLButtonElement).disabled).toBe(true);
      await act(async () => {
        finish();
        await Promise.resolve();
      });
      advance(settle);
      expect(visible()).toEqual([]);
    });
  });
});
