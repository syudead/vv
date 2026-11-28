import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyScanNoticeSession,
  readScanNoticeSession,
  writeScanNoticeSession,
} from "./scanNoticeSession";

describe("scan notice session", () => {
  afterEach(() => {
    window.sessionStorage.clear();
    vi.restoreAllMocks();
  });

  it("returns empty state for missing and corrupt values", () => {
    expect(readScanNoticeSession()).toEqual(emptyScanNoticeSession());
    window.sessionStorage.setItem("vv.scan-notice", "broken");
    expect(readScanNoticeSession()).toEqual(emptyScanNoticeSession());
  });

  it("returns empty state when storage access throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(readScanNoticeSession()).toEqual(emptyScanNoticeSession());
  });

  it("does not throw when persistence is unavailable", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(() =>
      writeScanNoticeSession({
        trackingScanId: 2,
        acknowledgedTerminalScanId: null,
        completionNotice: {
          scanId: 2,
          expiresAt: Date.now() + 1000,
          pausedRemainingMs: null,
        },
      }),
    ).not.toThrow();
  });

  it("restores old notices and persisted paused remaining time", () => {
    window.sessionStorage.setItem(
      "vv.scan-notice",
      JSON.stringify({
        version: 1,
        trackingScanId: 2,
        acknowledgedTerminalScanId: null,
        completionNotice: { scanId: 2, expiresAt: 1000 },
      }),
    );
    expect(readScanNoticeSession().completionNotice?.pausedRemainingMs).toBeNull();

    writeScanNoticeSession({
      trackingScanId: 2,
      acknowledgedTerminalScanId: null,
      completionNotice: {
        scanId: 2,
        expiresAt: 1000,
        pausedRemainingMs: 600,
      },
    });
    expect(readScanNoticeSession().completionNotice?.pausedRemainingMs).toBe(600);
  });
});
