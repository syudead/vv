import { render, screen, waitFor, within } from "@testing-library/react";
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
  const state = values.state ?? "done";
  return {
    id: 2,
    status: state,
    videos: { total: 0, settled: 0 },
    issues: { failed: 0, substituted: 0, revision: 0 },
    state,
    settledAt: state === "done" ? "2026-09-28T06:04:00Z" : undefined,
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
    expect(screen.getByText("No changed files were found.")).toBeDefined();
    expect(screen.getByText(/^Finished /)).toBeDefined();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows an indeterminate bar and unknown count while targets are discovered", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ state: "running", status: "finding", videos: undefined })),
      ),
    );
    const { container } = renderSection();

    const progress = await screen.findByRole("progressbar", {
      name: "Progress of the videos in this scan",
    });
    expect(progress.getAttribute("aria-valuenow")).toBeNull();
    expect(screen.getByText("Looking for files…")).toBeDefined();
    expect(container.querySelector("section")?.textContent).not.toMatch(/\d/);
  });

  it("shows one video count, its meaning and the current activity while running", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : String(input).startsWith("/api/scans/current/issues")
            ? json({ scanId: 2, items: [] })
            : json(
                scan({
                  state: "done",
                  status: "running",
                  videos: { total: 10, settled: 7 },
                  issues: { failed: 1, substituted: 2, revision: 4 },
                  activity: { kind: "preview", fileName: "clip.mp4" },
                }),
              ),
      ),
    );
    const { container } = renderSection();

    expect(await screen.findByText("7 of 10 videos done")).toBeDefined();
    expect(
      screen.getByText(
        "Counts changed files and videos that still needed preparing, not the whole library.",
      ),
    ).toBeDefined();
    expect(screen.getByText("Creating the preview · clip.mp4")).toBeDefined();
    const headingRow = screen.getByRole("heading", {
      name: "Scan status",
    }).parentElement!;
    expect(within(headingRow).getByText("1 failed")).toBeDefined();
    expect(within(headingRow).getByText("2 to check")).toBeDefined();
    const progress = screen.getByRole("progressbar");
    expect(progress.getAttribute("aria-valuetext")).toBe("7 of 10 videos done");
    // 概要にない数字の欄や時刻の欄を置かない。
    const text = container.textContent ?? "";
    for (const removed of [
      "Processed",
      "Total",
      "Preparation left",
      "Started",
      "Finished",
    ]) {
      expect(text).not.toContain(removed);
    }
    expect(container.querySelector("dl")).toBeNull();
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
          json(
            {
              id: 4,
              status: "finding",
              issues: { failed: 0, substituted: 0, revision: 0 },
              state: "running",
            },
            202,
          ),
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
    expect(screen.getByText("The scan couldn't finish.")).toBeDefined();
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
    expect(await screen.findByText("The scan failed.")).toBeDefined();
    expect(container.textContent).not.toMatch(/[\u3040-\u30ff\u4e00-\u9fff]/);
  });

  it.each([
    ["done", scan({ state: "done", videos: { total: 3, settled: 3 } })],
    [
      "running",
      scan({
        state: "running",
        videos: { total: 10, settled: 4 },
        issues: { failed: 1, substituted: 1, revision: 1 },
        activity: {
          kind: "registering",
          fileName: "夏.mp4",
          folder: { rootId: 1, path: "旅行", rootName: "media" },
        },
      }),
    ],
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
          : String(input).startsWith("/api/scans/current/issues")
            ? json({
                scanId: current.id,
                items: [
                  {
                    severity: "failed",
                    kinds: ["unreadable", "thumbnail_failed"],
                    fileName: "夏.mp4",
                    folder: { rootId: 1, path: "旅行", rootName: "media" },
                    videoId: 5,
                  },
                ],
              })
            : json(current),
      ),
    );
    const { container } = renderSection();
    await screen.findByRole("heading", {
      name: `${PSEUDO_OPEN}Scan status${PSEUDO_CLOSE}`,
    });
    await waitFor(() => expect(container.textContent).toContain("⟦Scan"));
    await waitFor(() =>
      expect(screen.getByTestId("scan-detail").textContent).not.toBe(""),
    );
    expectCatalogTextOnly(container, ["/media/動画/sub", "夏.mp4", "media / 旅行"]);
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
