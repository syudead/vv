import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { ScanProvider } from "../shell/ScanProvider";
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

function renderSection() {
  return render(
    <MemoryRouter initialEntries={["/settings#scan-status"]}>
      <ScanProvider>
        <ScanStatusSection />
      </ScanProvider>
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
});
