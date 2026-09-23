import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
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
      <ScanProvider>
        <ScanNoticeProvider>
          <ScanProgressIndicator />
          <LocationProbe />
        </ScanNoticeProvider>
      </ScanProvider>
    </MemoryRouter>,
  );
}

describe("ScanProgressIndicator", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    window.sessionStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => vi.unstubAllGlobals());

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
    expect((await screen.findAllByRole("status"))[0]?.textContent).toContain("4 / 10 件");
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
    expect(await screen.findByRole("status")).toBeDefined();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});
