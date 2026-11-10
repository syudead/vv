import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BackgroundGroup, BackgroundWork } from "../api/background";
import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { deferred, json } from "../testing/autoImportServer";
import { OwnerAudience } from "../testing/audience";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import BackgroundPausedIndicator from "./BackgroundPausedIndicator";
import { BackgroundProvider } from "./BackgroundProvider";
import { ScanNoticeProvider } from "./ScanNoticeProvider";
import ScanProgressIndicator from "./ScanProgressIndicator";
import { ScanProvider } from "./ScanProvider";
import { writeDismissedScanId } from "./scanNoticeSession";

const statusURL = "/api/background";
const pauseURL = "/api/background/pause";

function groups(overrides: Partial<Record<string, Partial<BackgroundGroup>>> = {}) {
  return (
    ["scan", "import", "auto_tag", "face", "version_judge", "backfill"] as const
  ).map((group) => ({ group, remaining: 0, ...overrides[group] }));
}

// The total leaves out the face group, whose feature is off.
const paused: BackgroundWork = {
  paused: true,
  running: [],
  groups: groups({
    scan: { remaining: 1200, waiting: true },
    auto_tag: { remaining: 34 },
    face: { remaining: 40, featureOff: true },
  }),
};

const runningScan: Scan = {
  id: 1,
  origin: "manual",
  status: "running",
  state: "running",
  videos: { total: 10, settled: 4 },
  issues: { failed: 0, substituted: 0, revision: 0 },
};

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname + location.hash}</span>;
}

function renderIndicators({ owner = true }: { owner?: boolean } = {}) {
  const tree: ReactNode = (
    <ScanProvider>
      <BackgroundProvider>
        <ScanNoticeProvider>
          <ScanProgressIndicator />
          <BackgroundPausedIndicator />
          <LocationProbe />
          {/* The page's own controls come before the portalled summary in document order. */}
          <button type="button">Page control</button>
        </ScanNoticeProvider>
      </BackgroundProvider>
    </ScanProvider>
  );
  return render(
    <MemoryRouter initialEntries={["/library"]}>
      <TooltipProvider>
        {owner ? <OwnerAudience>{tree}</OwnerAudience> : tree}
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const pausedName =
  "Background work is paused, 1,234 left. Open the background work settings";

describe("BackgroundPausedIndicator", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let current: BackgroundWork;

  beforeEach(() => {
    window.sessionStorage.clear();
    window.localStorage.clear();
    current = paused;
    fetchMock.mockReset();
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === statusURL) return Promise.resolve(json(current));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      return Promise.resolve(json(runningScan));
    });
    vi.stubGlobal("fetch", fetchMock);
    installFakeEventSource();
  });
  afterEach(() => vi.unstubAllGlobals());

  it("shows the paused state and the total in place of a hidden scan, and opens the section", async () => {
    // The owner hid this scan before the pause; the paused indicator still shows.
    writeDismissedScanId(runningScan.id);
    const user = userEvent.setup();
    renderIndicators();

    const trigger = await screen.findByRole("button", { name: pausedName });
    expect(trigger.textContent).toBe("Paused1,2341,234 left");
    expect(screen.queryByRole("button", { name: "Hide the scan progress" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();

    await user.click(trigger);
    expect(screen.getByTestId("location").textContent).toBe(
      "/settings/scan#background-work",
    );
  });

  it("lists the groups in its summary and resumes from its switch", async () => {
    const user = userEvent.setup();
    renderIndicators();
    const trigger = await screen.findByRole("button", { name: pausedName });
    expect(screen.queryByRole("button", { name: /^Scanning/ })).toBeNull();

    // Tab from the indicator reaches the summary's switch, and the summary stays open.
    act(() => trigger.focus());
    const summary = await screen.findByRole("dialog");
    expect(within(summary).getByText("Background work is paused")).toBeDefined();
    expect(
      within(within(summary).getByLabelText("Left to do"))
        .getAllByRole("term")
        .map((term) => term.textContent),
    ).toEqual([
      "ScanWaiting for resume",
      "Import",
      "Auto-tagging",
      "Face recognitionOff",
      "Duplicate check",
      "Catch-up",
    ]);
    await user.tab();
    const pauseSwitch = within(summary).getByRole("switch", {
      name: "Pause background work",
    });
    expect(document.activeElement).toBe(pauseSwitch);
    expect(screen.getByRole("dialog")).toBe(summary);

    const answer = deferred<Response>();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === pauseURL && init?.method === "PUT") return answer.promise;
      if (url === statusURL) return Promise.resolve(json(current));
      return Promise.resolve(json(runningScan));
    });
    await user.click(pauseSwitch);
    expect(
      fetchMock.mock.calls
        .filter(([input]) => String(input) === pauseURL)
        .map(([, init]) => init?.body),
    ).toEqual([JSON.stringify({ paused: false })]);
    expect(within(summary).getByText("Resuming…")).toBeDefined();

    current = { ...paused, paused: false };
    await act(async () => answer.resolve(json(current)));
    expect(
      screen.queryByRole("button", { name: /Background work is paused/ }),
    ).toBeNull();
    expect(
      await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ }),
    ).toBeDefined();
  });

  it("keeps the summary open while focus is on its switch and the pointer leaves", async () => {
    const user = userEvent.setup();
    renderIndicators();
    const trigger = await screen.findByRole("button", { name: pausedName });
    act(() => trigger.focus());
    const summary = await screen.findByRole("dialog");
    await user.tab();
    const pauseSwitch = within(summary).getByRole("switch", {
      name: "Pause background work",
    });
    expect(document.activeElement).toBe(pauseSwitch);

    vi.useFakeTimers();
    try {
      fireEvent.pointerEnter(summary);
      fireEvent.pointerLeave(summary);
      await act(async () => vi.advanceTimersByTimeAsync(200));
      expect(screen.getByRole("dialog")).toBe(summary);
    } finally {
      vi.useRealTimers();
    }

    // Focus leaving the summary closes it once the pointer is gone.
    act(() => screen.getByRole("button", { name: "Page control" }).focus());
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("does not show a scan result that came while paused, after the resume", async () => {
    let scanNow = runningScan;
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === statusURL) return Promise.resolve(json(current));
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      return Promise.resolve(json(scanNow));
    });
    renderIndicators();
    await screen.findByRole("button", { name: pausedName });

    scanNow = {
      ...runningScan,
      status: "done",
      state: "done",
      settledAt: "2026-11-10T06:04:00Z",
    };
    await emitServerEvent("scan", scanNow);
    current = { ...paused, paused: false };
    await emitServerEvent("background");
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /Background work is paused/ }),
      ).toBeNull(),
    );
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
  });

  it("is not shown to a guest, who never reads the status", async () => {
    renderIndicators({ owner: false });
    await act(async () => undefined);
    expect(
      screen.queryByRole("button", { name: /Background work is paused/ }),
    ).toBeNull();
    expect(fetchMock.mock.calls.some(([input]) => String(input) === statusURL)).toBe(
      false,
    );
  });
});
