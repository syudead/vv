import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { OwnerAudience } from "../testing/audience";
import { ScanNoticeProvider, useScanNotice } from "./ScanNoticeProvider";
import { forgetFailureKeys } from "./scanIssueMemory";
import { ScanProvider, useScan } from "./ScanProvider";

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(id: number, state: Scan["state"], values: Partial<Scan> = {}): Scan {
  return {
    id,
    origin: "manual",
    status: state,
    videos: { total: 1, settled: state === "running" ? 0 : 1 },
    issues: { failed: 0, substituted: 0, revision: 0 },
    state,
    ...values,
  };
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
    <OwnerAudience>
      <ScanProvider>
        <ScanNoticeProvider>
          <Harness />
        </ScanNoticeProvider>
      </ScanProvider>
    </OwnerAudience>,
  );
}

describe("ScanNoticeProvider", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    forgetFailureKeys();
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

  it("expires a failed scan notice like a completed one", async () => {
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

    expect(screen.getByTestId("notice").textContent).toBe("none");
  });

  it("expires a restored failed notice only after the current scan loads", async () => {
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
    expect(screen.getByTestId("notice").textContent).toBe("none");
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

  it("走査が閉じても準備が残る（status が running）あいだは、完了を知らせない", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan(21, "running")),
      ),
    );
    renderProvider();
    await waitFor(() => expect(screen.getByTestId("tracking").textContent).toBe("21"));

    await emitServerEvent("scan", scan(21, "done", { status: "running" }));
    expect(screen.getByTestId("notice").textContent).toBe("none");

    await emitServerEvent("scan", scan(21, "done"));
    await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("21"));
  });

  it("一部失敗の通知も、完了と同じく時間がたてば消え、止めた間は消えない", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json([{}]) : json(scan(22, "running")),
      ),
    );
    vi.useFakeTimers();
    renderProvider();
    await act(async () => Promise.resolve());
    await emitServerEvent(
      "scan",
      scan(22, "done", {
        status: "partial",
        issues: { failed: 2, substituted: 0, revision: 1 },
      }),
    );
    expect(screen.getByTestId("notice").textContent).toBe("22");

    await act(async () => screen.getByRole("button", { name: "pause" }).click());
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByTestId("notice").textContent).toBe("22");

    await act(async () => screen.getByRole("button", { name: "resume" }).click());
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(screen.getByTestId("notice").textContent).toBe("none");
  });

  describe("自動の取り込み（origin watch）", () => {
    const watch = (id: number, state: Scan["state"], values: Partial<Scan> = {}) =>
      scan(id, state, { origin: "watch", ...values });
    const partial = (id: number, failed: number) =>
      watch(id, "done", {
        status: "partial",
        issues: { failed, substituted: 0, revision: id },
      });

    /** files は今の取り込みの問題の一覧（失敗の動画のファイル名）である。 */
    function serve(current: () => { id: number; files: string[] }) {
      fetchMock.mockImplementation((input) => {
        const url = String(input);
        if (url === "/api/media-folders") return Promise.resolve(json([{}]));
        if (url.startsWith("/api/scans/current/issues")) {
          const { id, files } = current();
          return Promise.resolve(
            json({
              scanId: id,
              items: files.map((fileName) => ({
                severity: "failed",
                kinds: ["probe_failed"],
                fileName,
                folder: { rootId: 1, path: "" },
              })),
            }),
          );
        }
        return Promise.resolve(json(scan(1, "done")));
      });
    }

    it("走っているあいだと、失敗の無い完了は通知しない", async () => {
      serve(() => ({ id: 1, files: [] }));
      renderProvider();
      await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("none"));

      await emitServerEvent("scan", watch(60, "running"));
      expect(screen.getByTestId("tracking").textContent).not.toBe("60");
      expect(screen.getByTestId("notice").textContent).toBe("none");

      await emitServerEvent("scan", watch(60, "done"));
      expect(screen.getByTestId("notice").textContent).toBe("none");
    });

    it("失敗で終わったものは通知する", async () => {
      serve(() => ({ id: 1, files: [] }));
      renderProvider();
      await waitFor(() =>
        expect(screen.getByTestId("tracking").textContent).toBeDefined(),
      );

      await emitServerEvent("scan", watch(61, "running"));
      await emitServerEvent("scan", watch(61, "failed", { status: "failed" }));

      await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("61"));
    });

    it("新しい失敗を伴う partial だけ通知し、前に読んだ失敗だけなら通知しない", async () => {
      let files = ["a.mp4"];
      let id = 62;
      serve(() => ({ id, files }));
      renderProvider();
      await waitFor(() =>
        expect(screen.getByTestId("tracking").textContent).toBeDefined(),
      );

      // 前に読んだ一覧が無いので通知する。
      await emitServerEvent("scan", partial(62, 1));
      await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("62"));
      await act(async () => screen.getByRole("button", { name: "acknowledge" }).click());
      expect(screen.getByTestId("notice").textContent).toBe("none");

      // 同じ失敗が持ち越されただけ。
      id = 63;
      await emitServerEvent("scan", partial(63, 1));
      await act(async () => new Promise((done) => setTimeout(done, 20)));
      expect(screen.getByTestId("notice").textContent).toBe("none");

      // 新しい失敗が足された。
      id = 64;
      files = ["a.mp4", "b.mp4"];
      await emitServerEvent("scan", partial(64, 2));
      await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("64"));
    });

    it("失敗で終わった取り込みの失敗も覚え、次の partial が持ち越しても通知しない", async () => {
      let id = 80;
      serve(() => ({ id, files: ["a.mp4"] }));
      renderProvider();
      await waitFor(() =>
        expect(screen.getByTestId("tracking").textContent).toBeDefined(),
      );

      await emitServerEvent("scan", watch(80, "running"));
      await emitServerEvent(
        "scan",
        watch(80, "failed", {
          status: "failed",
          issues: { failed: 1, substituted: 0, revision: 80 },
        }),
      );
      await waitFor(() => expect(screen.getByTestId("notice").textContent).toBe("80"));
      await act(async () => new Promise((done) => setTimeout(done, 20)));
      await act(async () => screen.getByRole("button", { name: "acknowledge" }).click());

      // 同じ a.mp4 が持ち越されただけの partial。
      id = 81;
      await emitServerEvent("scan", partial(81, 1));
      await act(async () => new Promise((done) => setTimeout(done, 20)));
      expect(screen.getByTestId("notice").textContent).toBe("none");
    });

    it("手動の取り込みで読んだ失敗は、次の自動の取り込みで通知しない", async () => {
      let id = 70;
      serve(() => ({ id, files: ["a.mp4"] }));
      renderProvider();
      await waitFor(() =>
        expect(screen.getByTestId("tracking").textContent).toBeDefined(),
      );

      await emitServerEvent("scan", scan(70, "running"));
      await emitServerEvent(
        "scan",
        scan(70, "done", {
          status: "partial",
          issues: { failed: 1, substituted: 0, revision: 1 },
        }),
      );
      await act(async () => new Promise((done) => setTimeout(done, 20)));
      await act(async () => screen.getByRole("button", { name: "acknowledge" }).click());

      id = 71;
      await emitServerEvent("scan", partial(71, 1));
      await act(async () => new Promise((done) => setTimeout(done, 20)));
      expect(screen.getByTestId("notice").textContent).toBe("none");
    });
  });
});
