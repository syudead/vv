import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { TooltipProvider } from "../ui/Tooltip";
import { ScanNoticeProvider } from "./ScanNoticeProvider";
import ScanProgressIndicator from "./ScanProgressIndicator";
import { ScanProvider } from "./ScanProvider";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(values: Partial<Scan> = {}): Scan {
  return {
    id: 1,
    state: "running",
    total: 10,
    completed: 4,
    failed: 0,
    ...values,
  };
}

function LocationProbe() {
  const location = useLocation();
  return (
    <span data-testid="location">
      {location.pathname}
      {location.hash}
    </span>
  );
}

function renderIndicator() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ScanProvider>
          <ScanNoticeProvider>
            <ScanProgressIndicator />
            <LocationProbe />
          </ScanNoticeProvider>
        </ScanProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("ScanProgressIndicator", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    window.sessionStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("does not show a historical scan until it is running or notified", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([]) : json({}, 404)),
    );
    renderIndicator();
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /取り込み状況を開く/ })).toBeNull(),
    );
  });

  it("shows shared progress and navigates to settings from the trigger", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", { name: /取り込み中 40%/ });

    await user.hover(trigger);
    expect((await screen.findByRole("dialog")).textContent).toContain("4 / 10 件");
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("40");
    await user.unhover(trigger);
    await user.click(trigger);
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/settings#scan-status"),
    );
  });

  it("keeps the summary closed after a focusless click navigates to settings", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const trigger = await screen.findByRole("button", { name: /取り込み中 40%/ });

    await act(async () => fireEvent.click(trigger));

    expect(screen.getByTestId("location").textContent).toBe("/settings#scan-status");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens the summary on focus and closes it with Escape", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", { name: /取り込み中 40%/ });
    await user.tab();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(await screen.findByRole("dialog")).toBeDefined();
    expect(document.activeElement).toBe(trigger);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("keeps the summary open while the pointer moves into its portalled content", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const trigger = await screen.findByRole("button", { name: /取り込み中 40%/ });

    fireEvent.pointerEnter(trigger);
    const dialog = await screen.findByRole("dialog");
    fireEvent.pointerLeave(trigger.closest(".fixed")!);
    fireEvent.pointerEnter(dialog);
    await new Promise((resolve) => window.setTimeout(resolve, 150));

    expect(screen.getByRole("dialog")).toBeDefined();
  });

  it("shows unknown totals with an indeterminate progress bar", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ total: 0, completed: 0 })),
      ),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", { name: /^取り込み中。/ });
    await user.hover(trigger);

    const progress = await screen.findByRole("progressbar", {
      name: "取り込み対象を確認中",
    });
    expect(progress.getAttribute("aria-valuenow")).toBeNull();
    expect(screen.getByRole("dialog").textContent).toContain("0 / 確認中 件");
  });

  it("acknowledges a failed notice before navigating to its details", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ state, error: state === "failed" ? "disk" : undefined })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    state = "failed";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", {
      name: /取り込みに失敗しました。取り込み状況を開く/,
    });

    fireEvent.click(trigger);

    expect(screen.getByTestId("location").textContent).toBe("/settings#scan-status");
    expect(screen.queryByRole("button", { name: /取り込み状況を開く/ })).toBeNull();
    expect(window.sessionStorage.getItem("vv.scan-notice")).toContain(
      '"acknowledgedTerminalScanId":1',
    );
  });

  it("keeps a long failure reason in settings instead of the summary", async () => {
    let state: Scan["state"] = "running";
    const reason = "storage endpoint ".repeat(80);
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ state, error: state === "failed" ? reason : undefined })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    state = "failed";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", {
      name: /取り込みに失敗しました。取り込み状況を開く/,
    });

    fireEvent.pointerEnter(trigger);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain("設定で理由を確認してください");
    expect(dialog.textContent).not.toContain(reason);
  });

  it("clears interaction state when a failed notice is dismissed", async () => {
    let current = scan();
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(current),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    current = scan({ state: "failed", error: "disk" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    const close = await screen.findByRole("button", {
      name: "取り込み失敗の通知を閉じる",
    });

    fireEvent.pointerEnter(close);
    fireEvent.click(close);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /取り込み状況を開く/ })).toBeNull(),
    );

    current = scan({ id: 2, state: "running" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    expect(screen.queryByRole("dialog")).toBeNull();

    vi.useFakeTimers();
    current = scan({ id: 2, state: "done", completed: 10 });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await screen.findByRole("button", { name: /^完了。/ });
    await act(async () => vi.advanceTimersByTimeAsync(8100));
    expect(screen.queryByRole("button", { name: /^完了。/ })).toBeNull();
  });

  it("pauses a completed notice while the real summary is hovered", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan({ state })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^完了。/ });
    vi.useFakeTimers();

    await act(async () => fireEvent.pointerEnter(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByRole("button", { name: /^完了。/ })).toBeDefined();

    await act(async () => fireEvent.pointerLeave(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    await act(async () => vi.advanceTimersByTimeAsync(8100));
    expect(screen.queryByRole("button", { name: /^完了。/ })).toBeNull();
  });

  it("pauses a completed notice while the real summary is focused", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan({ state })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^完了。/ });
    vi.useFakeTimers();

    await act(async () => fireEvent.focus(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByRole("button", { name: /^完了。/ })).toBeDefined();

    await act(async () => fireEvent.blur(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(8000));
    expect(screen.queryByRole("button", { name: /^完了。/ })).toBeNull();
  });

  it("preserves a hovered completion notice's remaining time across reload", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan({ state })),
      ),
    );
    const first = renderIndicator();
    await screen.findByRole("button", { name: /取り込み中 40%/ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^完了。/ });
    vi.useFakeTimers();

    await act(async () => fireEvent.pointerEnter(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(10000));
    first.unmount();
    renderIndicator();
    await act(async () => Promise.resolve());
    await screen.findByRole("button", { name: /^完了。/ });

    await act(async () => vi.advanceTimersByTimeAsync(7900));
    expect(screen.getByRole("button", { name: /^完了。/ })).toBeDefined();
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(screen.queryByRole("button", { name: /^完了。/ })).toBeNull();
  });
});
