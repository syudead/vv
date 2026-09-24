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
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
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
    await act(async () => vi.advanceTimersByTimeAsync(8000));
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
});
