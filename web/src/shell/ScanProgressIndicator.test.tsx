import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { OwnerAudience } from "../testing/audience";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ScanNoticeProvider } from "./ScanNoticeProvider";
import ScanProgressIndicator from "./ScanProgressIndicator";
import { ScanProvider, useScan } from "./ScanProvider";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * scan は取り込みの応答である。state だけを指定したときは、status をそれに合わせる
 * （走査が閉じて準備も終わった形）。
 */
function scan(values: Partial<Scan> = {}): Scan {
  const state = values.state ?? "running";
  const status: Scan["status"] = values.status ?? state;
  return {
    id: 1,
    origin: "manual",
    status,
    videos: { total: 10, settled: status === "running" ? 4 : 10 },
    issues: { failed: 0, substituted: 0, revision: 0 },
    state,
    settledAt:
      status === "done" || status === "partial" ? "2026-09-28T06:04:00Z" : undefined,
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

function StartButton() {
  const scan = useScan();
  return (
    <button type="button" onClick={scan.start} disabled={!scan.canStart}>
      start for test
    </button>
  );
}

function renderIndicator() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <OwnerAudience>
          <ScanProvider>
            <ScanNoticeProvider>
              <ScanProgressIndicator />
              <LocationProbe />
              <StartButton />
            </ScanNoticeProvider>
          </ScanProvider>
        </OwnerAudience>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

describe("ScanProgressIndicator", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    window.sessionStorage.clear();
    window.localStorage.clear();
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
    installFakeEventSource();
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
      expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull(),
    );
  });

  it("shows shared progress and navigates to settings from the trigger", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    expect(trigger.textContent).toBe("Scanning4 / 10");

    await user.hover(trigger);
    expect((await screen.findByRole("dialog")).textContent).toContain(
      "4 of 10 videos done",
    );
    const progress = screen.getByRole("progressbar", {
      name: "Progress of the videos in this scan",
    });
    expect(progress.getAttribute("aria-valuenow")).toBe("4");
    expect(progress.getAttribute("aria-valuemax")).toBe("10");
    expect(progress.getAttribute("aria-valuetext")).toBe("4 of 10 videos done");
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
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

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
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
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
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    fireEvent.pointerEnter(trigger);
    const dialog = await screen.findByRole("dialog");
    fireEvent.pointerLeave(trigger.closest(".fixed")!);
    fireEvent.pointerEnter(dialog);
    await new Promise((resolve) => window.setTimeout(resolve, 150));

    expect(screen.getByRole("dialog")).toBeDefined();
  });

  it("closes the hovered summary when the pointer leaves and does not reopen it", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    await user.hover(trigger);
    expect(await screen.findByRole("dialog")).toBeDefined();
    await user.unhover(trigger);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).not.toBe(trigger);

    await user.hover(screen.getByTestId("location"));
    await new Promise((resolve) => window.setTimeout(resolve, 150));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes the hovered summary with Escape and keeps it closed", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    fireEvent.pointerEnter(trigger);
    expect(await screen.findByRole("dialog")).toBeDefined();
    await act(async () => fireEvent.keyDown(document.body, { key: "Escape" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await act(async () => fireEvent.pointerMove(trigger));
    await new Promise((resolve) => window.setTimeout(resolve, 150));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes the summary on a press outside it", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    const user = userEvent.setup();
    await act(async () => fireEvent.focus(trigger));
    expect(await screen.findByRole("dialog")).toBeDefined();
    // 外を押したことの受け取りは、開いた次の tick から始まる。
    await new Promise((resolve) => window.setTimeout(resolve, 0));
    await user.click(screen.getByTestId("location"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("closes the summary when the trigger moves to settings", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    await user.hover(trigger);
    expect(await screen.findByRole("dialog")).toBeDefined();
    await user.click(trigger);
    expect(screen.getByTestId("location").textContent).toBe("/settings#scan-status");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(trigger);
    await new Promise((resolve) => window.setTimeout(resolve, 150));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps a keyboard-opened summary closed after Escape until focus leaves and returns", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });

    await user.tab();
    expect(await screen.findByRole("dialog")).toBeDefined();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    // 閉じたことで本体へフォーカスが戻っても（focus が届いても）開き直らない。
    await act(async () => fireEvent.focus(trigger));
    expect(screen.queryByRole("dialog")).toBeNull();

    // フォーカスが本体から外れると閉じ、戻れば開く。
    await user.tab();
    expect(document.activeElement).not.toBe(trigger);
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(trigger);
    expect(await screen.findByRole("dialog")).toBeDefined();
    await user.tab();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("closes the indicator during a scan and keeps it hidden through the result", async () => {
    let current = scan();
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(current),
      ),
    );
    const first = renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });

    fireEvent.click(screen.getByRole("button", { name: "Hide the scan progress" }));
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
    expect(window.localStorage.getItem("vv.scan-indicator-dismissed")).toContain(
      '"scanId":1',
    );

    // 読み込み直しと別のタブ（sessionStorage が空）でも、同じ取り込みは出さない。
    first.unmount();
    window.sessionStorage.clear();
    renderIndicator();
    await act(async () => window.dispatchEvent(new Event("focus")));
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();

    for (const result of [
      scan({ state: "done" }),
      scan({
        state: "done",
        status: "partial",
        issues: { failed: 1, substituted: 0, revision: 1 },
      }),
      scan({ state: "failed", error: "disk" }),
    ]) {
      current = result;
      await emitServerEvent("scan", result);
      expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
      // 結果の通知も読み上げもしない。
      expect(screen.queryByRole("status")).toBeNull();
    }

    // 次の取り込みは、また出す。
    current = scan({ id: 2 });
    await emitServerEvent("scan", current);
    expect(
      await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ }),
    ).toBeDefined();
  });

  it("closes a scan that is still starting and shows the next one again", async () => {
    let current: Scan | null = null;
    let resolveStart: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/scans" && init?.method === "POST") {
        return new Promise<Response>((resolve) => {
          resolveStart = resolve;
        });
      }
      return Promise.resolve(current === null ? json({}, 404) : json(current));
    });
    renderIndicator();
    const user = userEvent.setup();
    const start = screen.getByRole("button", { name: "start for test" });
    await waitFor(() => expect(start.hasAttribute("disabled")).toBe(false));

    await user.click(start);
    await user.click(
      await screen.findByRole("button", { name: "Hide the scan progress" }),
    );
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();

    current = scan({ id: 5 });
    await act(async () => resolveStart?.(json(current, 202)));
    await act(async () => Promise.resolve());
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
    expect(window.localStorage.getItem("vv.scan-indicator-dismissed")).toContain(
      '"scanId":5',
    );

    current = scan({ id: 6 });
    await emitServerEvent("scan", current);
    expect(
      await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ }),
    ).toBeDefined();
  });

  it("keeps a scan hidden while starting when only the start response is lost", async () => {
    let current: Scan | null = null;
    let rejectStart: ((reason: unknown) => void) | undefined;
    let starts = 0;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/scans" && init?.method === "POST") {
        starts += 1;
        if (starts === 1) {
          return new Promise<Response>((_, reject) => {
            rejectStart = reject;
          });
        }
        return Promise.resolve(json(scan({ id: 8 }), 202));
      }
      return Promise.resolve(current === null ? json({}, 404) : json(current));
    });
    renderIndicator();
    const user = userEvent.setup();
    const start = screen.getByRole("button", { name: "start for test" });
    await waitFor(() => expect(start.hasAttribute("disabled")).toBe(false));

    await user.click(start);
    await user.click(
      await screen.findByRole("button", { name: "Hide the scan progress" }),
    );
    await act(async () => rejectStart?.(new TypeError("network")));

    // サーバーは取り込みを始めていた。あとの知らせで見つかっても出さない。
    current = scan({ id: 7 });
    await emitServerEvent("scan", current);
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
    expect(window.localStorage.getItem("vv.scan-indicator-dismissed")).toContain(
      '"scanId":7',
    );

    // 次の開始の取り込みは出す。
    current = scan({ id: 7, state: "done" });
    await emitServerEvent("scan", current);
    await user.click(start);
    current = scan({ id: 8 });
    await emitServerEvent("scan", current);
    expect(
      await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ }),
    ).toBeDefined();
  });

  it("shows the next scan when a hidden start really failed", async () => {
    let current: Scan | null = null;
    let rejectStart: ((reason: unknown) => void) | undefined;
    let starts = 0;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}]));
      if (url === "/api/scans" && init?.method === "POST") {
        starts += 1;
        if (starts === 1) {
          return new Promise<Response>((_, reject) => {
            rejectStart = reject;
          });
        }
        return Promise.resolve(json(scan({ id: 3 }), 202));
      }
      return Promise.resolve(current === null ? json({}, 404) : json(current));
    });
    renderIndicator();
    const user = userEvent.setup();
    const start = screen.getByRole("button", { name: "start for test" });
    await waitFor(() => expect(start.hasAttribute("disabled")).toBe(false));

    await user.click(start);
    await user.click(
      await screen.findByRole("button", { name: "Hide the scan progress" }),
    );
    await act(async () => rejectStart?.(new TypeError("network")));
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();

    current = scan({ id: 3 });
    await user.click(start);
    expect(
      await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ }),
    ).toBeDefined();
  });

  it("hides the indicator when another tab closes it", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });

    const newValue = JSON.stringify({ version: 1, scanId: 1 });
    window.localStorage.setItem("vv.scan-indicator-dismissed", newValue);
    await act(async () =>
      window.dispatchEvent(
        new StorageEvent("storage", { key: "vv.scan-indicator-dismissed", newValue }),
      ),
    );
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
  });

  it("lets a completed notice expire after the summary was opened and closed", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan({ state })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^Done / });
    vi.useFakeTimers();

    await act(async () => fireEvent.pointerEnter(trigger));
    expect(screen.getByRole("dialog")).toBeDefined();
    await act(async () => fireEvent.keyDown(document.body, { key: "Escape" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    // ポインタは本体の上に残っていても、閉じた概要は通知を止めない。
    await act(async () => vi.advanceTimersByTimeAsync(8100));
    expect(screen.queryByRole("button", { name: /^Done / })).toBeNull();
  });

  it("finding のあいだは割合も数字も出さず、不確定のバーにする", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ status: "finding", videos: undefined })),
      ),
    );
    renderIndicator();
    const user = userEvent.setup();
    const trigger = await screen.findByRole("button", { name: /^Scanning\./ });
    expect(trigger.textContent).toBe("Scanning");
    await user.hover(trigger);

    const progress = await screen.findByRole("progressbar", {
      name: "Progress of the videos in this scan",
    });
    expect(progress.getAttribute("aria-valuenow")).toBeNull();
    const dialog = screen.getByRole("dialog");
    expect(dialog.textContent).toContain("Looking for files…");
    expect(dialog.textContent).not.toMatch(/\d/);
  });

  it("ends a failed notice when moving to its details", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ state, error: state === "failed" ? "disk" : undefined })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    state = "failed";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", {
      name: /^Scan failed .*\. Open the scan status$/,
    });

    fireEvent.click(trigger);

    expect(screen.getByTestId("location").textContent).toBe("/settings#scan-status");
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
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
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    state = "failed";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", {
      name: /^Scan failed .*\. Open the scan status$/,
    });

    fireEvent.pointerEnter(trigger);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain("The scan couldn't finish.");
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
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    current = scan({ state: "failed", error: "disk" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    const close = await screen.findByRole("button", {
      name: "Hide the scan progress",
    });

    fireEvent.pointerEnter(close);
    fireEvent.click(close);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull(),
    );

    current = scan({ id: 2, state: "running" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    expect(screen.queryByRole("dialog")).toBeNull();

    vi.useFakeTimers();
    current = scan({ id: 2, state: "done" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await screen.findByRole("button", { name: /^Done / });
    await act(async () => vi.advanceTimersByTimeAsync(8100));
    expect(screen.queryByRole("button", { name: /^Done / })).toBeNull();
  });

  it("pauses a completed notice while the real summary is hovered", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan({ state })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^Done / });
    vi.useFakeTimers();

    await act(async () => fireEvent.pointerEnter(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByRole("button", { name: /^Done / })).toBeDefined();

    await act(async () => fireEvent.pointerLeave(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(100));
    await act(async () => vi.advanceTimersByTimeAsync(8100));
    expect(screen.queryByRole("button", { name: /^Done / })).toBeNull();
  });

  it("pauses a completed notice while the real summary is focused", async () => {
    let state: Scan["state"] = "running";
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan({ state })),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^Done / });
    vi.useFakeTimers();

    await act(async () => fireEvent.focus(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByRole("button", { name: /^Done / })).toBeDefined();

    await act(async () => fireEvent.blur(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(8000));
    expect(screen.queryByRole("button", { name: /^Done / })).toBeNull();
  });

  it("preserves a hovered completion notice's remaining time across reload", async () => {
    let state: Scan["state"] = "running";
    let delayCurrentScan = false;
    let resolveCurrentScan: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (delayCurrentScan) {
        return new Promise<Response>((resolve) => {
          resolveCurrentScan = resolve;
        });
      }
      return Promise.resolve(json(scan({ state })));
    });
    const first = renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });
    state = "done";
    await act(async () => window.dispatchEvent(new Event("focus")));
    const trigger = await screen.findByRole("button", { name: /^Done / });
    vi.useFakeTimers();

    await act(async () => vi.advanceTimersByTimeAsync(2000));
    await act(async () => fireEvent.pointerEnter(trigger));
    await act(async () => vi.advanceTimersByTimeAsync(10000));
    const pausedRemainingMs = JSON.parse(
      window.sessionStorage.getItem("vv.scan-notice") ?? "{}",
    ).completionNotice?.pausedRemainingMs as number;
    expect(pausedRemainingMs).toBeGreaterThan(5900);
    expect(pausedRemainingMs).toBeLessThanOrEqual(6000);
    first.unmount();
    delayCurrentScan = true;
    renderIndicator();
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(7000));
    expect(
      JSON.parse(window.sessionStorage.getItem("vv.scan-notice") ?? "{}").completionNotice
        ?.pausedRemainingMs,
    ).toBe(pausedRemainingMs);

    await act(async () => resolveCurrentScan?.(json(scan({ state: "done" }))));
    await screen.findByRole("button", { name: /^Done / });

    await act(async () => vi.advanceTimersByTimeAsync(pausedRemainingMs - 100));
    expect(screen.getByRole("button", { name: /^Done / })).toBeDefined();
    await act(async () => vi.advanceTimersByTimeAsync(100));
    expect(screen.queryByRole("button", { name: /^Done / })).toBeNull();
  });

  it("走査が閉じても準備が残るあいだは Scanning のままで、完了と表示しない", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([{}]) : json(scan())),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });

    await emitServerEvent(
      "scan",
      scan({ state: "done", status: "running", videos: { total: 10, settled: 9 } }),
    );
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 9 of 10 videos done\./,
    });
    expect(screen.queryByRole("button", { name: /^Done/ })).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("");

    // 仕事の件数や段階ごとの内訳は無い。
    await act(async () => fireEvent.focus(trigger));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).queryByRole("term")).toBeNull();
    expect(dialog.textContent).not.toMatch(/Analysis|Thumbnails|Previews|left/);
    await act(async () => fireEvent.blur(trigger));

    await emitServerEvent("scan", scan({ state: "done" }));
    expect(
      await screen.findByRole("button", { name: /^Done 10 of 10 videos done\./ }),
    ).toBeDefined();
    expect(screen.getByRole("status").textContent).toBe("The scan is complete.");
  });

  it("今の処理が変わっても、読み上げと行の配置は変わらない", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan({ activity: { kind: "probe", fileName: "a.mp4" } })),
      ),
    );
    renderIndicator();
    const trigger = await screen.findByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
    await act(async () => fireEvent.focus(trigger));
    const line = await screen.findByTestId("scan-detail");
    expect(line.textContent).toBe("Analyzing · a.mp4");
    const status = screen.getByRole("status");
    const className = line.className;

    await emitServerEvent(
      "scan",
      scan({
        videos: { total: 10, settled: 5 },
        activity: {
          kind: "preview",
          fileName: `${"very long file name ".repeat(10)}.mp4`,
          folder: { rootId: 1, path: "a/b", rootName: "media" },
        },
      }),
    );
    expect(screen.getByTestId("scan-detail")).toBe(line);
    expect(line.textContent).toMatch(/^Creating the preview · very long/);
    expect(line.className).toBe(className);
    expect(line.className).toContain("truncate");
    expect(line.getAttribute("title")).toMatch(/^media \/ a\/b \/ very long/);
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");

    // activity が一瞬無くなっても、行を空にしない。
    await emitServerEvent("scan", scan({ videos: { total: 10, settled: 6 } }));
    expect(line.textContent).toMatch(/^Creating the preview/);
    expect(status.textContent).toBe("");
  });

  it("一部失敗は失敗の本数と読み上げを示し、時間がたてば自動で閉じる", async () => {
    let current = scan();
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(current),
      ),
    );
    renderIndicator();
    await screen.findByRole("button", { name: /^Scanning 4 of 10 videos done\./ });

    vi.useFakeTimers();
    current = scan({
      state: "done",
      status: "partial",
      issues: { failed: 2, substituted: 1, revision: 3 },
    });
    await emitServerEvent("scan", current);
    const trigger = screen.getByRole("button", {
      name: "Some failed 10 of 10 videos done, 2 failed and 1 to check. Open the scan status",
    });
    // 本体には失敗だけを出す。
    expect(trigger.textContent).toContain("2 failed");
    expect(trigger.textContent).not.toContain("to check");
    expect(screen.getByRole("status").textContent).toBe(
      "The scan finished with some failures. 2 videos may not be usable.",
    );

    await act(async () => fireEvent.focus(trigger));
    const dialog = screen.getByRole("dialog");
    expect(dialog.textContent).toContain("2 failed");
    expect(dialog.textContent).toContain("1 to check");
    expect(dialog.textContent).toContain("Finished ");
    expect(dialog.textContent).toContain("See Settings for the list.");
    await act(async () => fireEvent.blur(trigger));

    await act(async () => vi.advanceTimersByTimeAsync(8100));
    expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
  });

  it("疑似ロケールで取り込み中・完了・一部失敗・失敗の表示はカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    let current = scan({
      activity: {
        kind: "seekThumbnail",
        fileName: "夏の旅行.mp4",
        folder: { rootId: 1, path: "旅行", rootName: "media" },
      },
    });
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(current),
      ),
    );
    renderIndicator();
    const open = async (name: RegExp) => {
      const trigger = await screen.findByRole("button", { name });
      await act(async () => fireEvent.focus(trigger));
      await screen.findByRole("dialog");
      expectCatalogTextOnly(document.body, [
        "/settings#scan-status",
        "/",
        "夏の旅行.mp4",
        "start for test",
      ]);
      await act(async () => fireEvent.blur(trigger));
    };
    await open(/Scanning/);

    current = scan({
      state: "done",
      status: "partial",
      issues: { failed: 1, substituted: 2, revision: 1 },
    });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await open(/Some failed/);

    current = scan({
      state: "failed",
      errorCode: "media_folder_unreadable",
      errorPath: "/media/動画",
    });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await open(/Scan failed/);
  });

  describe("origin watch", () => {
    function serve(failedFiles: string[]) {
      fetchMock.mockImplementation((input) => {
        const url = String(input);
        if (url === "/api/media-folders") return Promise.resolve(json([{}]));
        if (url.startsWith("/api/scans/current/issues")) {
          return Promise.resolve(
            json({
              scanId: 9,
              items: failedFiles.map((fileName) => ({
                severity: "failed",
                kinds: ["probe_failed"],
                fileName,
                folder: { rootId: 1, path: "" },
              })),
            }),
          );
        }
        return Promise.resolve(json({}, 404));
      });
    }

    it("shows nothing while a watch scan runs, and nothing when it ends done", async () => {
      serve([]);
      renderIndicator();
      await waitFor(() => expect(fetchMock).toHaveBeenCalled());

      await emitServerEvent("scan", scan({ id: 9, origin: "watch", state: "running" }));
      expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
      expect(screen.queryByRole("status")?.textContent ?? "").toBe("");

      await emitServerEvent("scan", scan({ id: 9, origin: "watch", state: "done" }));
      expect(screen.queryByRole("button", { name: /Open the scan status/ })).toBeNull();
    });

    it("shows a result for a new failure, with the auto-import words, and announces it", async () => {
      serve(["a.mp4"]);
      renderIndicator();
      await waitFor(() => expect(fetchMock).toHaveBeenCalled());

      await emitServerEvent("scan", scan({ id: 9, origin: "watch", state: "running" }));
      await emitServerEvent(
        "scan",
        scan({
          id: 9,
          origin: "watch",
          state: "done",
          status: "partial",
          issues: { failed: 1, substituted: 0, revision: 1 },
        }),
      );

      const trigger = await screen.findByRole("button", {
        name: /^Auto-import: some failed, 1 failed\. Open the scan status$/,
      });
      expect(trigger.textContent).toContain("Auto-import: some failed");
      expect(trigger.textContent).not.toContain("/");
      expect(
        screen
          .getAllByRole("status")
          .some((node) =>
            node.textContent?.startsWith(
              "Auto-import finished with some failures. 1 video may not be usable.",
            ),
          ),
      ).toBe(true);
    });

    it("shows a result for a failed watch scan", async () => {
      serve([]);
      renderIndicator();
      await waitFor(() => expect(fetchMock).toHaveBeenCalled());

      await emitServerEvent("scan", scan({ id: 9, origin: "watch", state: "running" }));
      await emitServerEvent(
        "scan",
        scan({ id: 9, origin: "watch", state: "failed", status: "failed" }),
      );

      expect(
        await screen.findByRole("button", {
          name: /^Auto-import failed\. Open the scan status$/,
        }),
      ).toBeDefined();
    });
  });
});
