import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { OwnerAudience } from "../testing/audience";
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

/**
 * scan は取り込みの応答である。state だけを指定したときは、status をそれに合わせる
 * （走査が閉じて準備も終わった形）。
 */
function scan(values: Partial<Scan> = {}): Scan {
  const state = values.state ?? "running";
  const status: Scan["status"] = values.status ?? state;
  return {
    id: 1,
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

function renderIndicator() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <OwnerAudience>
          <ScanProvider>
            <ScanNoticeProvider>
              <ScanProgressIndicator />
              <LocationProbe />
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
      name: "Dismiss the scan result notice",
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

  it("一部失敗は閉じるまで残り、失敗の本数と読み上げを示す", async () => {
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

    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByRole("button", { name: /^Some failed/ })).toBeDefined();

    await act(async () => fireEvent.focus(trigger));
    const dialog = screen.getByRole("dialog");
    expect(dialog.textContent).toContain("2 failed");
    expect(dialog.textContent).toContain("1 to check");
    expect(dialog.textContent).toContain("Finished ");
    expect(dialog.textContent).toContain("See Settings for the list.");
    await act(async () => fireEvent.blur(trigger));

    fireEvent.click(
      screen.getByRole("button", { name: "Dismiss the scan result notice" }),
    );
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
});
