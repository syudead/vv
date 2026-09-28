import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import {
  enablePseudoLocale,
  expectCatalogTextOnly,
  PSEUDO_CLOSE,
  PSEUDO_OPEN,
} from "../i18n/pseudo";
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
  return {
    id: 2,
    status: "done",
    state: "done",
    total: 0,
    completed: 0,
    failed: 0,
    ...values,
  };
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
    const heading = await screen.findByRole("heading", { name: "Scan status" });
    expect(document.activeElement).toBe(heading);
    expect(screen.getByText("There was nothing to scan")).toBeDefined();
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
      name: "Checking what to scan",
    });
    expect(progress.getAttribute("aria-valuenow")).toBeNull();
    expect(screen.getByText("Counting…")).toBeDefined();
  });

  it("shows retry text without a progress bar when no scan has loaded", async () => {
    fetchMock.mockImplementation((input) =>
      String(input) === "/api/media-folders"
        ? Promise.resolve(json([{}]))
        : Promise.reject(new Error("network")),
    );
    renderSection();

    expect(
      await screen.findByText(
        "Couldn't reach the server. Check that VVMDM is running and try again.",
      ),
    ).toBeDefined();
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
        json(
          scan({
            state: "failed",
            error: "open /media/動画: permission denied",
            errorCode: "media_folder_unreadable",
            errorPath: "/media/動画",
          }),
        ),
      );
    });
    renderSection();
    expect(
      await screen.findByText("The media folder couldn't be read: /media/動画"),
    ).toBeDefined();
    expect(screen.queryByText(/permission denied/)).toBeNull();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(startCalls).toBe(1));
  });

  it("shows a general English reason for an old failure without a code", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(
              scan({
                state: "failed",
                error: "取り込みの途中でアプリケーションが停止しました",
              }),
            ),
      ),
    );
    const { container } = renderSection();
    expect((await screen.findAllByText("The scan failed.")).length).toBeGreaterThan(0);
    expect(container.textContent).not.toMatch(/[\u3040-\u30ff\u4e00-\u9fff]/);
  });

  it.each([
    ["done", scan({ state: "done", total: 3, completed: 3 })],
    ["running", scan({ state: "running", total: 10, completed: 4, failed: 1 })],
    [
      "failed",
      scan({
        state: "failed",
        errorCode: "location_unreadable",
        errorPath: "/media/動画/sub",
      }),
    ],
    ["failed without a code", scan({ state: "failed", error: "古い理由" })],
  ])("renders only catalog text when %s (pseudo-locale)", async (_, current) => {
    enablePseudoLocale();
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json({
              ...current,
              startedAt: "2026-09-27T10:00:00Z",
              finishedAt:
                current.state === "running" ? undefined : "2026-09-27T10:05:00Z",
            }),
      ),
    );
    const { container } = renderSection();
    await screen.findByRole("heading", {
      name: `${PSEUDO_OPEN}Scan status${PSEUDO_CLOSE}`,
    });
    await waitFor(() => expect(container.textContent).toContain("Started"));
    expectCatalogTextOnly(container, ["/media/動画/sub"]);
  });

  it("focuses the heading again when navigating to the same anchor", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([]) : json(scan())),
    );
    renderSection({ navigation: true });
    const heading = await screen.findByRole("heading", { name: "Scan status" });
    const user = userEvent.setup();
    const navigation = screen.getByRole("button", { name: "同じ詳細へ移動" });

    await user.click(navigation);

    await waitFor(() => expect(document.activeElement).toBe(heading));
  });
});
