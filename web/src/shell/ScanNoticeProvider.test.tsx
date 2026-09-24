import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
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
      <button type="button" onClick={() => notice.setCompletionNoticePaused(true)}>
        pause
      </button>
      <button type="button" onClick={() => notice.setCompletionNoticePaused(false)}>
        resume
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
    installFakeEventSource();
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
    await emitServerEvent("scan", scan(8, "failed"));
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
    await emitServerEvent("scan", scan(11, "failed"));
    expect(screen.getByTestId("notice").textContent).toBe("11");

    await act(async () => vi.advanceTimersByTimeAsync(9000));

    expect(screen.getByTestId("notice").textContent).toBe("11");
  });

  it("does not expire a restored failed notice before the current scan loads", async () => {
    vi.useFakeTimers();
    window.sessionStorage.setItem(
      "vv.scan-notice",
      JSON.stringify({
        version: 1,
        trackingScanId: 14,
        acknowledgedTerminalScanId: null,
        completionNotice: { scanId: 14, expiresAt: Date.now() - 1000 },
      }),
    );
    let resolveScan: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      return new Promise<Response>((resolve) => {
        resolveScan = resolve;
      });
    });

    renderProvider();
    expect(screen.getByTestId("notice").textContent).toBe("14");
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByTestId("notice").textContent).toBe("14");

    await act(async () => resolveScan?.(json(scan(14, "failed"))));
    expect(screen.getByTestId("notice").textContent).toBe("14");
  });

  it("does not let an expired old notice overwrite a newly running scan", async () => {
    window.sessionStorage.setItem(
      "vv.scan-notice",
      JSON.stringify({
        version: 1,
        trackingScanId: 14,
        acknowledgedTerminalScanId: null,
        completionNotice: { scanId: 14, expiresAt: Date.now() - 1000 },
      }),
    );
    let response = scan(15, "running");
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(response),
      ),
    );

    renderProvider();
    await waitFor(() => expect(screen.getByTestId("tracking").textContent).toBe("15"));
    expect(screen.getByTestId("notice").textContent).toBe("none");

    response = scan(15, "done");
    await act(async () => screen.getByRole("button", { name: "refresh" }).click());
    await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("15"));
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
    await emitServerEvent("scan", scan(12, "done"));
    expect(screen.getByTestId("notice").textContent).toBe("12");

    await act(async () => vi.advanceTimersByTimeAsync(8000));

    expect(screen.getByTestId("notice").textContent).toBe("none");
  });

  it("pauses a completed scan notice while it is being read", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan(13, currentCalls++ === 0 ? "running" : "done")),
      ),
    );
    vi.useFakeTimers();
    renderProvider();
    await act(async () => Promise.resolve());
    await emitServerEvent("scan", scan(13, "done"));
    expect(screen.getByTestId("notice").textContent).toBe("13");

    await act(async () => screen.getByRole("button", { name: "pause" }).click());
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByTestId("notice").textContent).toBe("13");

    await act(async () => screen.getByRole("button", { name: "resume" }).click());
    await act(async () => vi.advanceTimersByTimeAsync(8000));
    expect(screen.getByTestId("notice").textContent).toBe("none");
  });

  it("restores the remaining time when a paused notice is reloaded", async () => {
    let response = scan(16, "running");
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(response),
      ),
    );
    vi.useFakeTimers();
    const first = renderProvider();
    await act(async () => Promise.resolve());
    response = scan(16, "done");
    await act(async () => screen.getByRole("button", { name: "refresh" }).click());
    expect(screen.getByTestId("notice").textContent).toBe("16");

    await act(async () => vi.advanceTimersByTimeAsync(2000));
    await act(async () => screen.getByRole("button", { name: "pause" }).click());
    expect(window.sessionStorage.getItem("vv.scan-notice")).toContain(
      '"pausedRemainingMs":6000',
    );
    await act(async () => vi.advanceTimersByTimeAsync(10000));
    first.unmount();
    renderProvider();
    await act(async () => Promise.resolve());
    expect(screen.getByTestId("notice").textContent).toBe("16");

    await act(async () => screen.getByRole("button", { name: "resume" }).click());
    await act(async () => vi.advanceTimersByTimeAsync(5900));
    expect(screen.getByTestId("notice").textContent).toBe("16");
    await act(async () => vi.advanceTimersByTimeAsync(100));
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

  it("準備の残りをまだ得ていない間は、完了を知らせない", async () => {
    let processing: Response | null = null;
    let resolveProcessing: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/processing") {
        if (processing !== null) return Promise.resolve(processing);
        return new Promise<Response>((resolve) => {
          resolveProcessing = resolve;
        });
      }
      return Promise.resolve(json(scan(21, "running")));
    });
    renderProvider();
    await waitFor(() => expect(screen.getByTestId("tracking").textContent).toBe("21"));

    await emitServerEvent("scan", scan(21, "done"));
    expect(screen.getByTestId("notice").textContent).toBe("none");

    await act(async () =>
      resolveProcessing?.(json({ probe: 0, thumbnail: 0, preview: 0 })),
    );
    processing = json({ probe: 0, thumbnail: 0, preview: 0 });
    await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("21"));
  });
});
