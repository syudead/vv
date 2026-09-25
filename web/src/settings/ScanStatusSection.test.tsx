import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { ScanProvider } from "../shell/ScanProvider";
import { OwnerAudience } from "../testing/audience";
import ScanStatusSection from "./ScanStatusSection";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(values: Partial<Scan> = {}): Scan {
  return { id: 2, state: "done", total: 0, completed: 0, failed: 0, ...values };
}

function SameAnchorNavigation() {
  const navigate = useNavigate();
  return (
    <button onClick={() => navigate("/settings#scan-status")}>同じ詳細へ移動</button>
  );
}

function renderSection({ navigation = false } = {}) {
  return render(
    <MemoryRouter initialEntries={["/settings#scan-status"]}>
      <OwnerAudience>
        <ScanProvider>
          {navigation && <SameAnchorNavigation />}
          <ScanStatusSection />
        </ScanProvider>
      </OwnerAudience>
    </MemoryRouter>,
  );
}

describe("ScanStatusSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => vi.unstubAllGlobals());

  it("shows the no-items result without a fake progress bar", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([]) : json(scan())),
    );
    renderSection();
    const heading = await screen.findByRole("heading", { name: "取り込み状況" });
    expect(document.activeElement).toBe(heading);
    expect(screen.getByText("対象はありませんでした")).toBeDefined();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows an indeterminate bar and unknown count while targets are discovered", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ state: "running" })),
      ),
    );
    renderSection();

    const progress = await screen.findByRole("progressbar", {
      name: "取り込み対象を確認中",
    });
    expect(progress.getAttribute("aria-valuenow")).toBeNull();
    expect(screen.getByText("確認中")).toBeDefined();
  });

  it("shows retry text without a progress bar when no scan has loaded", async () => {
    fetchMock.mockImplementation((input) =>
      String(input) === "/api/media-folders"
        ? Promise.resolve(json([{}]))
        : Promise.reject(new Error("network")),
    );
    renderSection();

    expect(await screen.findByText("network")).toBeDefined();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows a failed reason and retries through the existing scan action", async () => {
    let startCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/scans" && init?.method === "POST") {
        startCalls += 1;
        return Promise.resolve(
          json({ id: 4, state: "running", total: 0, completed: 0, failed: 0 }, 202),
        );
      }
      return Promise.resolve(
        json(scan({ state: "failed", error: "ディスクを読み取れません" })),
      );
    });
    renderSection();
    expect(await screen.findByText("ディスクを読み取れません")).toBeDefined();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "再試行" }));
    await waitFor(() => expect(startCalls).toBe(1));
  });

  it("focuses the heading again when navigating to the same anchor", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([]) : json(scan())),
    );
    renderSection({ navigation: true });
    const heading = await screen.findByRole("heading", { name: "取り込み状況" });
    const user = userEvent.setup();
    const navigation = screen.getByRole("button", { name: "同じ詳細へ移動" });

    await user.click(navigation);

    await waitFor(() => expect(document.activeElement).toBe(heading));
  });
});
