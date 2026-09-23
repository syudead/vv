import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { ScanNoticeProvider, useScanNotice } from "./ScanNoticeProvider";
import { ScanProvider, useScan } from "./ScanProvider";

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(id: number, state: Scan["state"]): Scan {
  return { id, state, total: 1, completed: state === "running" ? 0 : 1, failed: 0 };
}

function Harness() {
  const notice = useScanNotice();
  const scan = useScan();
  return (
    <>
      <p data-testid="tracking">{notice.trackingScanId ?? "none"}</p>
      <p data-testid="notice">{notice.completionNotice?.scanId ?? "none"}</p>
      <button type="button" onClick={notice.acknowledgeTerminalScan}>
        acknowledge
      </button>
      <button type="button" onClick={scan.refresh}>
        refresh
      </button>
    </>
  );
}

function renderProvider() {
  return render(
    <ScanProvider>
      <ScanNoticeProvider>
        <Harness />
      </ScanNoticeProvider>
    </ScanProvider>,
  );
}

describe("ScanNoticeProvider", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    window.sessionStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("keeps tracking and notice state across child replacement and remount", async () => {
    let response = scan(7, "running");
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(response),
      ),
    );
    const first = renderProvider();
    await waitFor(() => expect(screen.getByTestId("tracking").textContent).toBe("7"));

    response = scan(7, "done");
    await act(async () => first.rerender(<div>route replacement</div>));
    first.unmount();
    renderProvider();

    await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("7"));
  });

  it("does not show an acknowledged failed scan again", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan(8, currentCalls++ === 0 ? "running" : "failed")),
      ),
    );
    vi.useFakeTimers();
    const first = renderProvider();
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(2000));
    await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("8"));
    await act(async () => screen.getByRole("button", { name: "acknowledge" }).click());
    first.unmount();
    renderProvider();
    await act(async () => Promise.resolve());
    expect(screen.getByTestId("notice").textContent).toBe("none");
  });

  it("keeps a failed scan notice until acknowledgement", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan(11, currentCalls++ === 0 ? "running" : "failed")),
      ),
    );
    vi.useFakeTimers();
    renderProvider();
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(2000));
    expect(screen.getByTestId("notice").textContent).toBe("11");

    await act(async () => vi.advanceTimersByTimeAsync(9000));

    expect(screen.getByTestId("notice").textContent).toBe("11");
  });

  it("expires completed scan notices", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan(12, currentCalls++ === 0 ? "running" : "done")),
      ),
    );
    vi.useFakeTimers();
    renderProvider();
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(2000));
    expect(screen.getByTestId("notice").textContent).toBe("12");

    await act(async () => vi.advanceTimersByTimeAsync(8000));

    expect(screen.getByTestId("notice").textContent).toBe("none");
  });

  it("clears A when B starts and assigns B a fresh deadline after reload", async () => {
    vi.useFakeTimers();
    let response = scan(9, "running");
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(response),
      ),
    );
    const first = renderProvider();
    await screen.findByText("9");
    response = scan(9, "done");
    await act(async () => screen.getByRole("button", { name: "refresh" }).click());
    response = scan(10, "running");
    await act(async () => screen.getByRole("button", { name: "refresh" }).click());
    expect(screen.getByTestId("notice").textContent).toBe("none");
    first.unmount();
    response = scan(10, "done");
    renderProvider();
    await act(async () => Promise.resolve());
    expect(screen.getByTestId("notice").textContent).toBe("10");
  });
});
